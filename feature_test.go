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

package goclone

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// Returns an error if the given command line doesn't match the expected
// command line for the given process.
func checkCommandLine(pid int, command string) error {
	cmdlineFile := fmt.Sprintf("/proc/%d/cmdline", pid)
	contents, err := ioutil.ReadFile(cmdlineFile)
	if err != nil {
		return err
	}
	arg1 := bytes.Split(contents, []byte{0})[0]
	if !bytes.Equal(arg1, []byte(command)) {
		return fmt.Errorf("Command line not correct: %s", string(contents))
	}
	return nil
}

// Waits for up to 5 seconds for the given command line to appear in the given
// pids proc file. This is necessary since we do not get a notification when
// the process has called exec().
func waitForProcessStart(pid int, command string) error {
	end := time.Now().Add(time.Second * 5)
	for time.Now().Before(end) {
		if checkCommandLine(pid, command) == nil {
			return nil
		}
		time.Sleep(time.Millisecond)
	}
	return fmt.Errorf("Process never had the correct command line.")
}

func TestFeaturePath(t *testing.T) {
	cmd := &Cmd{
		Path: sleepBin,
		Args: []string{sleepBin, "60"},
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	defer cmd.Wait()
	defer cmd.Process.Kill()

	if err := waitForProcessStart(cmd.Process.Pid, sleepBin); err != nil {
		t.Fatalf("%s", err)
	}

	// Read the full command line and make sure that its valid.
	cmdlineFile := fmt.Sprintf("/proc/%d/cmdline", cmd.Process.Pid)
	cmdline, err := ioutil.ReadFile(cmdlineFile)
	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	want := bytes.Join([][]byte{
		[]byte(sleepBin),
		[]byte("60"),
		[]byte(""),
	}, []byte{0})
	if !bytes.Equal(cmdline, want) {
		t.Fatalf("Command line not correct: %#v", cmdline)
	}
}

func TestFeaturePathArgsDifference(t *testing.T) {
	falseName := "false_bin_name"
	cmd := &Cmd{
		Path: sleepBin,
		Args: []string{falseName, "60"},
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	defer cmd.Wait()
	defer cmd.Process.Kill()

	if err := waitForProcessStart(cmd.Process.Pid, falseName); err != nil {
		t.Fatalf("%s", err)
	}

	// Read the full command line and make sure that its valid.
	cmdlineFile := fmt.Sprintf("/proc/%d/cmdline", cmd.Process.Pid)
	cmdline, err := ioutil.ReadFile(cmdlineFile)
	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	want := bytes.Join([][]byte{
		[]byte(falseName),
		[]byte("60"),
		[]byte(""),
	}, []byte{0})
	if !bytes.Equal(cmdline, want) {
		t.Fatalf("Command line not correct: %#v", cmdline)
	}
}

func TestFeatureEnv(t *testing.T) {
	cmd := Command(sleepBin, "60")
	cmd.Env = []string{"A=B", "B=C", "TEST=TEST"}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	defer cmd.Wait()
	defer cmd.Process.Kill()

	if err := waitForProcessStart(cmd.Process.Pid, sleepBin); err != nil {
		t.Fatalf("%s", err)
	}

	// Read the processes environment.
	environFile := fmt.Sprintf("/proc/%d/environ", cmd.Process.Pid)
	environ, err := ioutil.ReadFile(environFile)
	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	env := bytes.Split(environ, []byte{0})
	if len(env) != len(cmd.Env)+1 {
		t.Fatalf("Environment variables are the wrong length: %d", len(env))
	}
	for i, kv := range env[0 : len(env)-1] {
		if string(kv) != cmd.Env[i] {
			t.Fatalf("Unexpected environment: %s", string(kv))
		}
	}
}

func TestFeatureDir(t *testing.T) {
	cmd := Command(sleepBin, "60")
	cmd.Dir = os.TempDir()
	if err := cmd.Start(); err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	defer cmd.Wait()
	defer cmd.Process.Kill()

	if err := waitForProcessStart(cmd.Process.Pid, sleepBin); err != nil {
		t.Fatalf("%s", err)
	}

	cwdFile := fmt.Sprintf("/proc/%d/cwd", cmd.Process.Pid)
	cwd, err := os.Readlink(cwdFile)
	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	if cwd != os.TempDir() {
		t.Fatalf("Process has the wrong cwd: %s", cwd)
	}
}

func TestFeatureStderr(t *testing.T) {
	cmd := Command(shBin, "-c", fmt.Sprintf("%s expected output >&2", echoBin))
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	defer cmd.Wait()
	defer cmd.Process.Kill()

	output, err := ioutil.ReadAll(stderr)
	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	} else if string(output) != "expected output\n" {
		t.Fatalf("Unexpected output: %s", string(output))
	}
}

func TestFeatureStdin(t *testing.T) {
	cmd := Command(catBin)
	cmd.Stdin = strings.NewReader("expected output")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	defer cmd.Wait()
	defer cmd.Process.Kill()

	output, err := ioutil.ReadAll(stdout)
	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	} else if string(output) != "expected output" {
		t.Fatalf("Unexpected output: %s", string(output))
	}
}

func TestFeatureStdout(t *testing.T) {
	cmd := Command(echoBin, "expected output")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	defer cmd.Wait()
	defer cmd.Process.Kill()

	output, err := ioutil.ReadAll(stdout)
	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	} else if string(output) != "expected output\n" {
		t.Fatalf("Unexpected output: %s", string(output))
	}
}

func TestFeatureExtraFiles(t *testing.T) {
	openFile := func() *os.File {
		fd, err := ioutil.TempFile("", "TestFeatureExtraFiles")
		if err != nil {
			t.Fatalf("Unexpected error: %s", err)
		}
		return fd
	}
	extraFiles := []*os.File{}
	defer func() {
		for _, fd := range extraFiles {
			if fd != nil {
				os.Remove(fd.Name())
				fd.Close()
			}
		}
	}()

	// Make a map of 5 extra files and some nils.
	extraFiles = append(extraFiles, openFile())
	extraFiles = append(extraFiles, nil)
	extraFiles = append(extraFiles, openFile())
	extraFiles = append(extraFiles, nil)
	extraFiles = append(extraFiles, openFile())
	extraFiles = append(extraFiles, nil)
	extraFiles = append(extraFiles, openFile())
	extraFiles = append(extraFiles, nil)
	extraFiles = append(extraFiles, openFile())

	cmd := Command(sleepBin, "60")
	cmd.ExtraFiles = extraFiles
	if err := cmd.Start(); err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	defer cmd.Wait()
	defer cmd.Process.Kill()

	if err := waitForProcessStart(cmd.Process.Pid, sleepBin); err != nil {
		t.Fatalf("%s", err)
	}

	fdDir := fmt.Sprintf("/proc/%d/fd", cmd.Process.Pid)
	dirs, err := ioutil.ReadDir(fdDir)
	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	fdMap := make(map[int]string)
	for _, dir := range dirs {
		link, err := os.Readlink(path.Join(fdDir, dir.Name()))
		if err != nil {
			t.Fatalf("Unexpected error: %s", err)
		}
		i, err := strconv.Atoi(dir.Name())
		if err != nil {
			t.Fatalf("Unexpected error: %s", err)
		}
		fdMap[i] = link
	}

	// Walk the map and verify that it looks the way it should.
	for fd, link := range fdMap {
		switch fd {
		case 0, 1, 2:
			if link != os.DevNull {
				t.Errorf(
					"%d should link to %s, links to %s instead",
					fd, link, os.DevNull)
			}
		case 3, 5, 7, 9, 11:
			if link != extraFiles[fd-3].Name() {
				t.Errorf(
					"%d should link to %s, links to %s instead",
					fd, extraFiles[fd-3].Name(), link)
			}
		default:
			t.Errorf("Unknown open file: %d (%s)", fd, link)
		}
	}
}

func TestFeatureSysProcAttrCredential(t *testing.T) {
	// Only run this test if the process is running as root (since we need to
	// be able to switch users for this.)
	if os.Getuid() != 0 {
		t.Skipf("This test must be run as root.")
	}

	cmd := Command(sleepBin, "60")
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	cmd.SysProcAttr.Credential = &syscall.Credential{}
	cmd.SysProcAttr.Credential.Uid = 100
	cmd.SysProcAttr.Credential.Gid = 101
	cmd.SysProcAttr.Credential.Groups = []uint32{102, 103, 104}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	defer cmd.Wait()
	defer cmd.Process.Kill()

	if err := waitForProcessStart(cmd.Process.Pid, sleepBin); err != nil {
		t.Fatalf("%s", err)
	}

	// Read the status file to see what the current working uid and gid of the
	// process are.
	statusFile := fmt.Sprintf("/proc/%d/status", cmd.Process.Pid)
	contents, err := ioutil.ReadFile(statusFile)
	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	for _, line := range bytes.Split(contents, []byte("\n")) {
		parts := bytes.Split(line, []byte(":"))
		if len(parts) != 2 {
			continue
		}
		switch string(parts[0]) {
		case "Uid":
			expect := []byte("\t100\t100\t100\t100")
			if !bytes.Equal(expect, parts[1]) {
				t.Fatalf("Unexpected Uid line value: %s", string(parts[1]))
			}
		case "Gid":
			expect := []byte("\t101\t101\t101\t101")
			if !bytes.Equal(expect, parts[1]) {
				t.Fatalf("Unexpected Gid line value: %s", string(parts[1]))
			}
		case "Groups":
			expect := []byte("\t102 103 104 ")
			if !bytes.Equal(expect, parts[1]) {
				t.Fatalf("Unexpected Groups line value: %s", string(parts[1]))
			}
		}
	}
}

func TestFeatureSysProcAttrChroot(t *testing.T) {
	// Check ti see if /bin/busybox exists, without it we will not have
	// a static binary to run in the root.
	sourceFd, err := os.Open("/bin/busybox")
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("Skipping due to a missing /bin/busybox")
		}
		t.Fatalf("Unknown error stating busybox: %s", err)
	}

	// Next we make a directory to "chroot" into, and copy the busybox binary
	// into it.
	mode := os.FileMode(0777)
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	dir, err := ioutil.TempDir("", "goclone")
	if err != nil {
		t.Fatalf("Error making temporary directory: %s", err)
	}
	defer func() {
		if err := os.RemoveAll(dir); err != nil {
			t.Fatalf("Error removing temp dir: %s", err)
		}
	}()
	if destFd, err := os.OpenFile(dir+"/busybox", flags, mode); err != nil {
		t.Fatalf("Error creating file: %s", err)
	} else if _, err := io.Copy(destFd, sourceFd); err != nil {
		t.Fatalf("Error copying file contents: %s", err)
	} else if err := sourceFd.Close(); err != nil {
		t.Fatalf("Error closing the source file: %s", err)
	} else if err := destFd.Close(); err != nil {
		t.Fatalf("Error closing the dest file: %s", err)
	}

	// Now we can attempt to execute the busybox binary in a chrooted
	// environment.
	cmd := Command("/busybox", "sleep", "10")
	cmd.SysProcAttr = &syscall.SysProcAttr{Chroot: dir}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Error starting the command: %s", err)
	}
	defer cmd.Wait()
	defer cmd.Process.Kill()

	// Wait for the process to fully start.
	if err := waitForProcessStart(cmd.Process.Pid, "/busybox"); err != nil {
		t.Fatalf("%s", err)
	}

	// Start with the link.. The link should be to the directory we created
	// above.
	root := fmt.Sprintf("/proc/%d/root", cmd.Process.Pid)
	if link, err := os.Readlink(root); err != nil {
		t.Fatalf("Error reading the root link: %s", err)
	} else if link != dir {
		t.Fatalf("Process had the wrong root, should be %s, is %s", dir, link)
	}
}

func TestFeatureSysProcAttrPtrace(t *testing.T) {
	t.Skip("TODO: Funtionality not implemented.")
}

func TestFeatureSysProcAttrSetsid(t *testing.T) {
	t.Skip("TODO: Funtionality not implemented.")
}

func TestFeatureSysProcAttrSetpgid(t *testing.T) {
	t.Skip("TODO: Funtionality not implemented.")
}

func TestFeatureSysProcAttrSetCtty(t *testing.T) {
	t.Skip("TODO: Funtionality not implemented.")
}

func TestFeatureSysProcAttrNotty(t *testing.T) {
	t.Skip("TODO: Funtionality not implemented.")
}

func TestFeatureSysProcAttrCtty(t *testing.T) {
	t.Skip("TODO: Funtionality not implemented.")
}

func TestFeatureSysProcAttrPdeathsig(t *testing.T) {
	t.Skip("FIXME: Test not implemented.")
}

func TestFeatureCgroupsTasksFiles(t *testing.T) {
	// Create some fake cgroups files so we don't need cgroups setup
	// or anything special.
	cgroupsTasksFiles := []string{}
	defer func() {
		for _, file := range cgroupsTasksFiles {
			if err := os.Remove(file); err != nil {
				t.Errorf("Unexpected error: %s", err)
			}
		}
	}()
	for i := 0; i < 5; i++ {
		fn, err := ioutil.TempFile("", "TestFeatureCgroupsTasksFiles")
		if err != nil {
			t.Fatalf("Unexpected error: %s", err)
		}
		cgroupsTasksFiles = append(cgroupsTasksFiles, fn.Name())
		if err := fn.Close(); err != nil {
			t.Fatalf("Unexpected error: %s", err)
		}
	}

	cmd := Command(trueBin)
	cmd.CgroupsTasksFiles = cgroupsTasksFiles
	if err := cmd.Run(); err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}

	// Check that the pid ended up in each of the cgroups tasks files.
	expected := []byte(fmt.Sprintf("%d\n", cmd.Process.Pid))
	for _, file := range cgroupsTasksFiles {
		if contents, err := ioutil.ReadFile(file); err != nil {
			t.Fatalf("Unexpected error: %s", err)
		} else if !bytes.Equal(expected, contents) {
			t.Fatalf("Unexpected contents: %s", contents)
		}
	}
}

func TestFeatureDoubleFork(t *testing.T) {
	t.Skipf("FIXME: Test not implemented.")
}

func TestFeatureIPCNameSpace(t *testing.T) {
	// Check to see if ipc namespaces are supported.
	if _, ok := SupportedNamespaces(t)["ipc"]; !ok {
		t.Skip("Kernel doesn't support ipc namespaces.")
	}

	t.Skipf("FIXME: Test not implemented.")
}

func TestFeatureMountNameSpace(t *testing.T) {
	// Check to see if mount namespaces are supported.
	if _, ok := SupportedNamespaces(t)["mount"]; !ok {
		t.Skip("Kernel doesn't support mount namespaces.")
	}

	t.Skipf("FIXME: Test not implemented.")
}

func TestFeatureNetworkNameSpace(t *testing.T) {
	// Check to see if network namespaces are supported.
	if _, ok := SupportedNamespaces(t)["net"]; !ok {
		t.Skip("Kernel doesn't support network namespaces.")
	}

	t.Skipf("FIXME: Test not implemented.")
}

func TestFeatureUserNameSpace(t *testing.T) {
	// Check to see if user namespaces are supported.
	if _, ok := SupportedNamespaces(t)["user"]; !ok {
		t.Skip("Kernel doesn't support user namespaces.")
	}

	t.Skipf("FIXME: Test not implemented.")
}

func TestFeatureUTSNameSpace(t *testing.T) {
	// Check to see if UTS namespaces are supported.
	if _, ok := SupportedNamespaces(t)["uts"]; !ok {
		t.Skip("Kernel doesn't support uts namespaces.")
	}

	t.Skipf("FIXME: Test not implemented.")
}

func TestFeatureNewIPCNameSpace(t *testing.T) {
	// Check to see if IPC namespaces are supported.
	if _, ok := SupportedNamespaces(t)["ipc"]; !ok {
		t.Skip("Kernel doesn't support IPC namespaces.")
	}

	t.Skipf("FIXME: Test not implemented.")
}

func TestFeatureNewMountNameSpace(t *testing.T) {
	// Check to see if mount namespaces are supported.
	if _, ok := SupportedNamespaces(t)["mount"]; !ok {
		t.Skip("Kernel doesn't support mount namespaces.")
	}

	t.Skipf("FIXME: Test not implemented.")
}

func TestFeatureNewNetworkNameSpace(t *testing.T) {
	// Check to see if network namespaces are supported.
	if _, ok := SupportedNamespaces(t)["network"]; !ok {
		t.Skip("Kernel doesn't support network namespaces.")
	}

	t.Skipf("FIXME: Test not implemented.")
}

func TestFeatureNewPIDNameSpace(t *testing.T) {
	// Check to see if PID namespaces are supported.
	if _, ok := SupportedNamespaces(t)["pid"]; !ok {
		t.Skip("Kernel doesn't support pid namespaces.")
	}

	t.Skipf("FIXME: Test not implemented.")
}

func TestFeatureNewUserNameSpace(t *testing.T) {
	// Check to see if user namespaces are supported.
	if _, ok := SupportedNamespaces(t)["user"]; !ok {
		t.Skip("Kernel doesn't support user namespaces.")
	}

	t.Skipf("FIXME: Test not implemented.")
}

func TestFeatureNewUTSNameSpace(t *testing.T) {
	// Check to see if UTC namespaces are supported.
	if _, ok := SupportedNamespaces(t)["utc"]; !ok {
		t.Skip("Kernel doesn't support utc namespaces.")
	}

	t.Skipf("FIXME: Test not implemented.")
}

func TestFeatureUserMap(t *testing.T) {
	// Check to see if user namespaces are supported.
	if _, ok := SupportedNamespaces(t)["user"]; !ok {
		t.Skip("Kernel doesn't support user namespaces.")
	}

	// Make a temporary directory.
	tmpDir, err := ioutil.TempDir("", "TestFeatureNewUserNameSpace")
	if err != nil {
		t.Fatalf("Error maping a temporary directory: %s", err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Fatalf("Error removing temp directory: %s", err)
		}
	}()

	// Make sure that the user can write to that temporary directory.
	if err := os.Chmod(tmpDir, 0777); err != nil {
		t.Fatalf("Error chmoding the temp directory: %s", err)
	}

	// Run a command that should touch a file in that temp directory as
	// root in the user name space.
	tmpFile := tmpDir + "/test"
	c := Command(touchBin, tmpFile)
	c.NewUserNameSpace = true
	c.UserMap = []MapElement{
		MapElement{Inside: 0, Outside: 1000, Length: 1},
	}
	c.SysProcAttr = &syscall.SysProcAttr{}
	c.SysProcAttr.Credential = &syscall.Credential{}
	c.SysProcAttr.Credential.Uid = 0
	c.SysProcAttr.Credential.Gid = 0
	if err := c.Start(); err != nil {
		t.Fatalf("Error running the command: %s", err)
	} else if err := c.Wait(); err != nil {
		t.Fatalf("Error waiting on the command: %s", err)
	}

	// See if the file was created.
	var stat syscall.Stat_t
	if err := syscall.Stat(tmpFile, &stat); err != nil {
		t.Fatalf("Error while stating the output file: %s", err)
	} else if stat.Uid != 1000 {
		t.Fatalf("User was not set properly, expected 1000, got %d", stat.Uid)
	}
}
func TestFeatureGroupMap(t *testing.T) {
	// Check to see if user namespaces are supported.
	if _, ok := SupportedNamespaces(t)["user"]; !ok {
		t.Skip("Kernel doesn't support user namespaces.")
	}

	t.Skip("FIXME: Not implemented.")
}
