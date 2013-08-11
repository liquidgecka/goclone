// Copyright 2013 Brady Catherman
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// +build linux

package main

import (
	"bytes"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/liquidgecka/goclone"
)

var (
	// A list of cgroups tasks files to join before executing the command.
	cgroupsTasksFiles = flag.String("cgroup_tasks_files", "", ""+
		"A list of cgroups tasks files that should have the"+
		"\n\tpid of the new process written into prior to calling exec()."+
		"\n\tThis ensures that all children of that process are in the"+
		"\n\tcgroup rather than having a brief race condition on startup."+
		"\n\tThis is a comma seperated list.")

	// The directory that should be chrooted into prior to calling exec.
	chroot = flag.String("chroot", "", ""+
		"Specifies a directory that should be chrooted into prior to"+
		"\n\tcalling exec. If this is used then --cwd and the binary path"+
		"\n\twill be relative to this directory.")

	// The working directory to give the new process.
	cwd = flag.String("cwd", "", ""+
		"Cwd to set before executing the process. If --chroot is defined"+
		"\n\tthen this will be relative to the chroot directory.")

	// Should the new process be double forked, therefor leaving it as a
	// child of initd and causing the goclone process to terminate right
	// away rather than waiting.
	doubleFork = flag.Bool("double_fork", false, ""+
		"Should goclone double fork before calling the child"+
		"\n\tprocess. This will leave the new process as a child of initd"+
		"\n\trather than glclone and will also mean that glclone will"+
		"\n\tterminate right away, leaving the new process running in the"+
		"\n\tbackground.")

	// Environment as passed in on the command line.
	env = flag.String("env", "", ""+
		"Environment variables to add, the format of this argument is"+
		"\n\tK1=V1,K2=V2")

	// Environment as passed in as a file name.
	envFile = flag.String("envfile", "", ""+
		"File to read environment variables from. The format of this file"+
		"\n\tis to have each environment variable on a single line, with a"+
		"\n\tformat of KEY=VALUE.")

	// Environment as passed in as a file name with null terminators. This
	// is typically used with /proc/<pid>/environ files.
	envFile0 = flag.String("envfile0", "", ""+
		"Null split File to read environment variables from. This is"+
		"\n\tmost typically used with /proc/<pid>/environ files in order to"+
		"\n\tcopy the environment from an existing process. This file works"+
		"\n\texactly like --envfile except that it is null seperated.")

	// If the user wants stdin to be a file rather than inheriting form this
	// this process they will use this flag.
	stdinFile = flag.String("stdin", "", ""+
		"The file to open and use for stdin rather than using the calling"+
		"\n\tprocesses stdin.")

	// Same for stdout.
	stdoutFile = flag.String("stdout", "", ""+
		"The file to open and use for stdout rather than using the calling"+
		"\n\tprocesses stdout.")

	// Same for stderr.
	stderrFile = flag.String("stderr", "", ""+
		"The file to open and use for stderr rather than using the calling"+
		"\n\tprocesses stderr.")

	// Join the given name spaces
	ipcNameSpace = flag.String("ipc_namespace", "", ""+
		"If specified the new process will be joined to the given"+
		"\n\tipc namespace. This should be in the form: /proc/<pid>/ns/ipc")
	mountNameSpace = flag.String("mount_namespace", "", ""+
		"If specified the new process will be joined to the given\n\t"+
		"mount namespace. This should be in the form: /proc/<pid>/ns/mnt")
	networkNameSpace = flag.String("network_namespace", "", ""+
		"If specified the new process will be joined to the"+
		"\n\tgiven network namespace. This should be in the form:"+
		"\n\t/proc/<pid>/ns/net")
	utsNameSpace = flag.String("uts_namespace", "", ""+
		"If specified the new process will be joined to the given\n\t"+
		"uts namespace. This should be in the form: /proc/<pid>/ns/uts")

	// New name spaces
	newIPCNameSpace = flag.Bool("new_ipc_namespace", false, ""+
		"If set the command will be started in a new IPC"+
		"\n\tnamespace.")
	newMountNameSpace = flag.Bool("new_mount_namespace", false, ""+
		"If set the command will be started in a new Mount"+
		"\n\tnamespace.")
	newNetworkNameSpace = flag.Bool("new_network_namespace", false, ""+
		"If set the command will be started in a new "+
		"\n\tNetwork namespace.")
	newPIDNameSpace = flag.Bool("new_pid_namespace", false, ""+
		"If set the command will be started in a new PID"+
		"\n\tnamespace.")
	newUTSNameSpace = flag.Bool("new_uts_namespace", false, ""+
		"If set the command will be started in a new UTS"+
		"\n\tnamespace.")
)

func fatal(f string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, f, args...)
	os.Exit(1)
}

func checkEnvPair(source string, pair string) {
	if len(pair) == 0 {
		fatal("Empty environment tuple in %s\n", source)
	}
	pairs := strings.SplitN(pair, "=", 1)
	if len(pairs) != 2 {
		fatal("Invalid environment tuple in %s. Expect K=V: %s\n", source, pair)
	}
	if len(pairs[0]) != 0 {
		fatal("Empty key in an environment tuple in %s: %s\n", source, pair)
	}
	for i, r := range pairs[0] {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '_':
		default:
			fatal("Invalid key character at index %s in tuple in %s: %s",
				i, source, pair)
		}
	}
}

func main() {
	// Setup the cmd object.
	cmd := goclone.Cmd{}

	// Parse the flags and start walking through them setting values.
	flag.Parse()

	// Get the command line.
	cmdline := flag.Args()
	if len(cmdline) == 0 {
		fatal("You must specify a command line to run.\n")
	}
	cmd.Path = cmdline[0]
	cmd.Args = cmdline

	// Cwd
	cmd.Dir = *cwd

	// Process the various environment sources.
	if *env != "" {
		for _, pair := range strings.Split(*env, ",") {
			checkEnvPair("--env", pair)
			cmd.Env = append(cmd.Env, pair)
		}
	}
	if *envFile != "" {
		contents, err := ioutil.ReadFile(*envFile)
		if err != nil {
			fatal("Unable to read environment file: %s\n", *envFile)
		}
		for _, pairBytes := range bytes.Split(contents, []byte(",")) {
			pair := string(pairBytes)
			checkEnvPair("--envfile", pair)
			cmd.Env = append(cmd.Env, pair)
		}
	}
	if *envFile0 != "" {
		contents, err := ioutil.ReadFile(*envFile)
		if err != nil {
			fatal("Unable to read environment file: %s\n", *envFile)
		}
		for _, pairBytes := range bytes.Split(contents, []byte{0}) {
			pair := string(pairBytes)
			checkEnvPair("--envfile0", pair)
			cmd.Env = append(cmd.Env, pair)
		}
	}

	// Stdin
	if *stdinFile != "" {
		stdin, err := os.Open(*stdinFile)
		if err != nil {
			fatal("Unable to open stdin file: %s\n", *stdinFile)
		}
		cmd.Stdin = stdin
	} else {
		cmd.Stdin = os.Stdin
	}

	// Stdout
	if *stdoutFile != "" {
		flags := os.O_WRONLY | os.O_APPEND | os.O_CREATE | os.O_TRUNC
		stdout, err := os.OpenFile(*stdoutFile, flags, 666)
		if err != nil {
			fatal("Unable to open stdout file: %s\n", *stdoutFile)
		}
		cmd.Stdout = stdout
	} else {
		cmd.Stdout = os.Stdout
	}

	// Stderr
	if *stderrFile != "" {
		if *stderrFile == *stdoutFile {
			cmd.Stderr = cmd.Stdout
		} else {
			flags := os.O_WRONLY | os.O_APPEND | os.O_CREATE | os.O_TRUNC
			stderr, err := os.OpenFile(*stderrFile, flags, 666)
			if err != nil {
				fatal("Unable to open stderr file: %s\n", *stderrFile)
			}
			cmd.Stderr = stderr
		}
	} else {
		cmd.Stderr = os.Stderr
	}

	// UID, GID, Groups
	// FIXME

	// Chroot
	if *chroot != "" {
		if cmd.SysProcAttr != nil {
			cmd.SysProcAttr = &syscall.SysProcAttr{}
		}
		cmd.SysProcAttr.Chroot = *chroot
	}

	// CgroupsTasksFiles
	if *cgroupsTasksFiles != "" {
		cmd.CgroupsTasksFiles = strings.Split(*cgroupsTasksFiles, ",")
		for _, file := range cmd.CgroupsTasksFiles {
			if _, err := os.Lstat(file); err != nil {
				fatal("Can not find cgroups tasks file: %s\n", file)
			}
		}
	}

	// DoubleFork
	cmd.DoubleFork = *doubleFork

	// IPCNameSpace
	cmd.IPCNameSpace = *ipcNameSpace
	if cmd.IPCNameSpace != "" {
		if _, err := os.Lstat(cmd.IPCNameSpace); err != nil {
			fatal("Can not find the ipc name space file: %s\n",
				cmd.IPCNameSpace)
		}
		if *newIPCNameSpace == true {
			fatal("Can not join an IPC namespace and make a new one.\n")
		}
	}

	// MountNameSpace
	cmd.MountNameSpace = *mountNameSpace
	if cmd.MountNameSpace != "" {
		if _, err := os.Lstat(cmd.MountNameSpace); err != nil {
			fatal("Can not find the mount name space file: %s\n",
				cmd.MountNameSpace)
		}
		if *newMountNameSpace == true {
			fatal("Can not join an Mount namespace and make a new one.\n")
		}
	}

	// NetworkNameSpace
	cmd.NetworkNameSpace = *networkNameSpace
	if cmd.NetworkNameSpace != "" {
		if _, err := os.Lstat(cmd.NetworkNameSpace); err != nil {
			fatal("Can not find the network name space file: %s\n",
				cmd.NetworkNameSpace)
		}
		if *newNetworkNameSpace == true {
			fatal("Can not join an Network namespace and make a new one.\n")
		}
	}

	// UTSNameSpace
	cmd.UTSNameSpace = *utsNameSpace
	if cmd.UTSNameSpace != "" {
		if _, err := os.Lstat(cmd.UTSNameSpace); err != nil {
			fatal("Can not find the uts name space file: %s\n",
				cmd.UTSNameSpace)
		}
		if *newUTSNameSpace == true {
			fatal("Can not join an UTS namespace and make a new one.\n")
		}
	}

	// New namespaces.
	cmd.NewIPCNameSpace = *newIPCNameSpace
	cmd.NewMountNameSpace = *newMountNameSpace
	cmd.NewNetworkNameSpace = *newNetworkNameSpace
	cmd.NewPIDNameSpace = *newPIDNameSpace
	cmd.NewUTSNameSpace = *newUTSNameSpace

	// Run the command.
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			waitStatus, ok := ee.ProcessState.Sys().(syscall.WaitStatus)
			if !ok {
				panic(fmt.Errorf(
					"Unknown type returned from ProcessState.Sys(): %#t",
					ee.ProcessState.Sys()))
			}
			os.Exit(waitStatus.ExitStatus())
		} else {
			fatal("Error running the command: %s\n", err)
		}
	}
}
