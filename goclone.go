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
	"os"
	"os/exec"
	"sync"
	"syscall"
)

// #include "goclone.h"
import "C"

// This is a structure that mirrors the basic model of os/exec.Cmd but enables
// a ton of extra very linux specific features via the "clone" system call.
//
// FIXME
type Cmd struct {
	// This is the path to the binary that should be executed.
	Path string

	// This is the list of arguments that will be passed to exec, including
	// the command name as Args[0].
	Args []string

	// This is the environment of the process. If this is nil then the
	// environment of the calling process will be used.
	Env []string

	// The working directory of the command. If Dir is empty then the calling
	// processes working directory will be used.
	Dir string

	// This is the io.Reader that will be used as the processes stdin. If this
	// is a os.File object then writing to the descriptor will be done
	// directly, otherwise a goroutine will be started to copy data from the
	// real descriptor to the internal reader.
	Stdin io.Reader

	// These are the io.Writers for Stdout, and Stderr. These work like
	// Stdin in that an os.File object is treated specially and will continue
	// to work even after the golang process has died.
	Stdout io.Writer
	Stderr io.Writer

	// This is a list of extra files that will be passed into the process.
	// these will be mapped in as file  descriptor 3+i. Unlike Stdin these
	// MUST be os.File objects.
	ExtraFiles []*os.File

	// This is a list of system specific attributes. This will be translated
	// into system specific functionally.
	SysProcAttr *syscall.SysProcAttr

	// This is the underlying Process once started.
	Process *os.Process

	// This it the processes state information about the running process.
	ProcessState *os.ProcessState

	// ------------------------
	// Goclone specific fields.
	// ------------------------

	// This is a list of cgroups tasks files that should have the new processes
	// pid written into. This is used to ensure that the new process is a
	// member of a specific set of cgroups. If any of these files can not
	// be written to then the child will not be executed.
	CgroupsTasksFiles []string

	// If this is set to true then the clone cycle will double fork. This
	// leaves the new process as a child of initd but means that Wait()
	// will return only when the first child exits, not the second.
	// If setting this it is best to just use Run().
	DoubleFork bool

	// These are name spaces that should be joined by the cloned child
	// before executing the process. This is either empty, or a string
	// of the form "/proc/123/ns/ipc". Note that its impossible to join
	// a PID name space so its not listed here.
	IPCNameSpace     string
	MountNameSpace   string
	NetworkNameSpace string
	UTSNameSpace     string
	PIDNameSpace     string

	// These boolean values are used to let the clone system know that it
	// should use the flags that specifically create new name spaces when
	// creating the child process.
	NewIPCNameSpace     bool
	NewMountNameSpace   bool
	NewNetworkNameSpace bool
	NewPIDNameSpace     bool
	NewUTSNameSpace     bool

	// ------------
	// Private Data
	// ------------

	// This mutex protects the data elements in this structure.
	mutex sync.Mutex

	// This is the list of file descriptors that need to be closed after
	// the process is executed.
	closeAfterStart []io.Closer
	closeAfterWait  []io.Closer

	// Set to true if the process has already been waited on.
	finished bool

	// If an error is encountered at any point in a child goroutine then this
	// field will be populated, otherwise it will be left as nil. Mutations
	// of this field should be done through the setError function which will
	// lock the mutex.
	err   error
	errWG sync.WaitGroup
}

// This is a helper function that will create a Cmd object with the given
// commandline pre-populated.
func Command(cmd string, args ...string) *Cmd {
	fullArgs := make([]string, len(args)+1)
	fullArgs[0] = cmd
	copy(fullArgs[1:], args)
	return &Cmd{
		Path: cmd,
		Args: fullArgs,
	}
}

// Returns stdout and stderr combined after running the process.
func (c *Cmd) CombinedOutput() ([]byte, error) {
	if c.Process != nil {
		return nil, fmt.Errorf("goclone: Process already started.")
	}
	if c.Stdout != nil {
		return nil, fmt.Errorf("goclone: Stdout already set.")
	}
	if c.Stderr != nil {
		return nil, fmt.Errorf("goclone: Stderr already set.")
	}
	buffer := &bytes.Buffer{}
	c.Stdout = buffer
	c.Stderr = buffer
	err := c.Run()
	return buffer.Bytes(), err
}

// Returns stdout as []bytes after running the process.
func (c *Cmd) Output() ([]byte, error) {
	if c.Process != nil {
		return nil, fmt.Errorf("goclone: Process already started.")
	}
	if c.Stdout != nil {
		return nil, fmt.Errorf("goclone: Stdout already set.")
	}
	buffer := &bytes.Buffer{}
	c.Stdout = buffer
	err := c.Run()
	return buffer.Bytes(), err
}

// This function wraps a call to Start() and then Wait()
func (c *Cmd) Run() error {
	if err := c.Start(); err != nil {
		return err
	}
	return c.Wait()
}

// Run is a simple wrapper around Start() then Wait() with a common err return.
func (c *Cmd) Start() (err error) {
	// If the process has already started then we can not continue here.
	if c.Process != nil {
		return fmt.Errorf("goclone: already started.")
	}

	// This function will walk through all of the file descriptors closing them
	// and returning the error passed in (or an error generated during the
	// Close() call if necessary.
	fail := func(err error) error {
		err = closeDescriptors(c.closeAfterStart, err)
		err = closeDescriptors(c.closeAfterWait, err)
		return err
	}

	// Setup the file descriptors needed to actually perform the syscall
	// level clone.
	stdin, err := c.reader(c.Stdin)
	if err != nil {
		return fail(err)
	}
	stdout, err := c.writer(c.Stdout)
	if err != nil {
		return fail(err)
	}

	// As a special case we have to check that stdout and stderr are not the
	// same object. If they are then we should use the same file descriptor
	// object rather than duplicating a whole new one.
	var stderr *os.File
	if c.Stdout == c.Stderr {
		stderr = stdout
	} else {
		stderr, err = c.writer(c.Stderr)
		if err != nil {
			return fail(err)
		}
	}

	// Make the native object type that is necessary in order to pass the
	// data into the cgo side of the world. We do this via a mmaped chunk
	// of memory so that we don't have to fiddle with malloc or any form
	// of shared memory allocations. Mmap works purely in pages so everything
	// needs to be sized to fit within pages of memory.
	//
	// The format for this allocation looks like this:
	// [ C.goclone_cmd ] - This will be several pages, rounded up to the
	//                     next page size.
	// [ buffer ] - This is necessary to ensure that we operate on sizes
	//              one page at a time.
	// [ page ] - This is a single page that will be memprotected in order to
	//            prevent the stack in the C side from growing large enough
	//            to cause memory corruption.
	// [ stack ] - This is a chunk of memory used as the stack for the cloned
	//             process.
	// [ page ] - This is another protected page.
	//
	// If double forking this will also be added to the allocation:
	// [ stack 2 ] - The stack for the short lived middle process.
	// [ page ] - Another protected page.

	// Start by calculating how much data needs to be available in the mmap
	// allocation.
	dataSize := rawDataSize(c)

	// Adjust the dataSize value to be a multiple of pageSize.
	dataSize = ((dataSize - 1) | (pageSize - 1)) + 1

	// Now calculate the total size of the allocation.
	size := dataSize + pageSize + stackSize + pageSize
	if c.DoubleFork {
		size += stackSize + pageSize
	}

	// Allocate the space and ensure it gets freed when the routine exits.
	m, err := mmapDataAlloc(size)
	if err != nil {
		return fail(err)
	}
	defer func() {
		err2 := m.free()
		if err == nil && err2 != nil {
			err = err2
		}
	}()

	// This boolean value is set if the credentials should be set at some
	// point. Its off by default unless a user defined the Credentials field
	// in the SysProcAttr field.
	setCredentials := false
	uid := uint32(0)
	gid := uint32(0)
	groups := []uint32(nil)
	if c.SysProcAttr != nil && c.SysProcAttr.Credential != nil {
		setCredentials = true
		uid = c.SysProcAttr.Credential.Uid
		gid = c.SysProcAttr.Credential.Gid
		groups = c.SysProcAttr.Credential.Groups
	}

	// If the c.SysProcAttr value is set then save the signal.
	deathSignal := syscall.SIGCHLD
	if c.SysProcAttr != nil && c.SysProcAttr.Pdeathsig != 0 {
		deathSignal = c.SysProcAttr.Pdeathsig
	}

	// The chroot directory is set from c.SysProcAttr if defined.
	chrootDir := ""
	if c.SysProcAttr != nil {
		chrootDir = c.SysProcAttr.Chroot
	}

	// Make an array of file descriptors that will be used to setup the
	// file descriptors in the child process.
	files := make([]int, len(c.ExtraFiles)+3)
	files[0] = int(stdin.Fd())
	files[1] = int(stdout.Fd())
	files[2] = int(stderr.Fd())
	for i, fd := range c.ExtraFiles {
		if fd != nil {
			files[i+3] = int(fd.Fd())
		} else {
			files[i+3] = -1
		}
	}

	// Populate the various data elements in the allocation.
	cmd := m.pushGocloneCmd()
	// Exec settings.
	cmd.path = m.pushString(c.Path)
	cmd.args = m.pushStringSlice(c.Args)
	cmd.env = m.pushStringSlice(c.Env)
	cmd.dir = m.pushString(c.Dir)
	cmd.chroot_dir = m.pushString(chrootDir)

	// file descriptors.
	cmd.files = m.pushIntSlice(files)
	cmd.files_len = C.int(len(files))

	// Credentials
	cmd.set_credentials = C.bool(setCredentials)
	cmd.uid = C.uid_t(uid)
	cmd.gid = C.gid_t(gid)
	cmd.groups = m.pushGidSlice(groups)
	cmd.groups_len = C.int(len(groups))

	// Cgroups tasks files.
	cmd.cgroups_tasks_files = m.pushStringSlice(c.CgroupsTasksFiles)

	// Namespaces
	cmd.ipc_namespace = m.pushString(c.IPCNameSpace)
	cmd.mount_namespace = m.pushString(c.MountNameSpace)
	cmd.network_namespace = m.pushString(c.NetworkNameSpace)
	cmd.uts_namespace = m.pushString(c.UTSNameSpace)
	cmd.pid_namespace = m.pushString(c.PIDNameSpace)
	cmd.new_ipc_namespace = C.bool(c.NewIPCNameSpace)
	cmd.new_network_namespace = C.bool(c.NewNetworkNameSpace)
	cmd.new_pid_namespace = C.bool(c.NewPIDNameSpace)
	cmd.new_mount_namespace = C.bool(c.NewMountNameSpace)
	cmd.new_uts_namespace = C.bool(c.NewUTSNameSpace)

	// Various simple settings.
	cmd.double_fork = C.bool(c.DoubleFork)
	cmd.death_signal = C.int(deathSignal)

	// Allocate the stacks.
	if err = m.mprotect(); err != nil {
		return
	}
	cmd.stack = m.stack()
	if err = m.mprotect(); err != nil {
		return
	}

	// If double forking, allocate another stack and protect the page after it.
	if c.DoubleFork {
		cmd.df_stack = m.stack()
		if err = m.mprotect(); err != nil {
			return
		}
	}

	// Call the C function.
	pid, err := C.goclone(cmd)

	// Close any file descriptors that are no longer needed.
	err = closeDescriptors(c.closeAfterStart, err)

	// Make the Process structure. This functions foot print returns an error
	// but on linux an error can never be returned.
	c.Process, _ = os.FindProcess(int(pid))

	return
}

// StderrPipe returns a pipe that will be connected to the commands stderr.
func (c *Cmd) StderrPipe() (io.ReadCloser, error) {
	if c.Process != nil {
		return nil, fmt.Errorf("goclone: Process already started.")
	}
	if c.Stderr != nil {
		return nil, fmt.Errorf("goclone: Stderr already set")
	}
	r, w, err := osPipe()
	if err != nil {
		return nil, err
	}
	c.Stderr = w
	c.closeAfterStart = append(c.closeAfterStart, w)
	c.closeAfterWait = append(c.closeAfterWait, r)
	return r, nil
}

// StdinPipe returns a pipe that will be connected to the commands stdin.
func (c *Cmd) StdinPipe() (io.WriteCloser, error) {
	if c.Process != nil {
		return nil, fmt.Errorf("goclone: Process already started.")
	}
	if c.Stdin != nil {
		return nil, fmt.Errorf("goclone: Stdin already set")
	}
	r, w, err := osPipe()
	if err != nil {
		return nil, err
	}
	c.Stdin = r
	c.closeAfterStart = append(c.closeAfterStart, r)
	c.closeAfterWait = append(c.closeAfterWait, w)
	return w, nil
}

// StdoutPipe returns a pipe that will be connected to the commands stdout.
func (c *Cmd) StdoutPipe() (io.ReadCloser, error) {
	if c.Process != nil {
		return nil, fmt.Errorf("goclone: Process already started.")
	}
	if c.Stdout != nil {
		return nil, fmt.Errorf("goclone: Stdout already set")
	}
	r, w, err := osPipe()
	if err != nil {
		return nil, err
	}
	c.Stdout = w
	c.closeAfterStart = append(c.closeAfterStart, w)
	c.closeAfterWait = append(c.closeAfterWait, r)
	return r, nil
}

// Waits for the process to finish. If Wait() has already been called then
// this will actually return an error.
func (c *Cmd) Wait() (err error) {
	if c.Process == nil {
		return fmt.Errorf("goclone: not started.")
	}

	// If the process already finished then don't bother actually waiting on it
	// since the syscall would wait on no process or some other process.
	setFinished := func() (finished bool) {
		c.mutex.Lock()
		defer c.mutex.Unlock()
		finished = c.finished
		c.finished = true
		return
	}()
	if setFinished {
		return fmt.Errorf("goclone: Wait was already called.")
	}

	// Actually wait on the process.
	c.ProcessState, err = c.Process.Wait()

	// Wait for all of the goroutines to finish. These should all close out
	// quickly now that the process has finished and been reaped.
	c.errWG.Wait()

	// If the error being returned is nil then check and see if a copy
	// goroutine encountered an error.
	if err == nil {
		c.mutex.Lock()
		err = c.err
		c.mutex.Unlock()
	}

	// Close all of the file descriptors that are supposed to be closed after
	// the wait finishes.
	err = closeDescriptors(c.closeAfterWait, err)

	// If the process exited with a non zero status then return a special
	// error type just like exec.Wait()
	if err == nil && !c.ProcessState.Success() {
		return &exec.ExitError{c.ProcessState}
	}

	return err
}

// -----------------
// Private Functions
// -----------------

// Closes all the file descriptors in the given list of closers.
func closeDescriptors(fds []io.Closer, err error) error {
	for _, fd := range fds {
		fd.Close()
	}
	return err
}

// Converts stdin into a os.File object if it is not already. This will create
// a background goroutine to shovel data back and forth if the object is not
// a os.File since this is necessary with the way io.Writers work. If the
// underlying object is actually an os.File object then no background goroutine
// is necessary.
func (c *Cmd) reader(reader io.Reader) (fd *os.File, err error) {
	// The reader is not defined.
	if reader == nil {
		fd, err = osOpen(os.DevNull)
		if err != nil {
			return
		}
		c.closeAfterStart = append(c.closeAfterStart, fd)
		return
	}

	// The reader is actually already an os.File object.
	var ok bool
	if fd, ok = reader.(*os.File); ok {
		return
	}

	// Stdin is not an os.File object. We need a goroutine to shovel data
	// between the real os.File object that we have to make in order for this
	// to work properly.
	fd, w, err := osPipe()
	if err != nil {
		return
	}

	// Add the file descriptors to the various tracking routines.
	c.closeAfterStart = append(c.closeAfterStart, fd)
	c.closeAfterWait = append(c.closeAfterWait, w)
	c.errWG.Add(1)
	go func() {
		_, err := io.Copy(w, reader)
		c.setError(err)
		c.setError(w.Close())
		c.errWG.Done()
	}()
	return
}

// This will set the background 'err' value to the given err value if the value
// is non nil.
func (c *Cmd) setError(err error) {
	if err == nil {
		return
	} else {
		c.mutex.Lock()
		defer c.mutex.Unlock()
		if c.err == nil {
			c.err = err
		}
	}
}

// This is a wrapper that can convert a io.Writer object to a os.File object
// in order to be used as stdout or stderr.
func (c *Cmd) writer(writer io.Writer) (fd *os.File, err error) {
	// If the writer is nil, then open dev null.
	if writer == nil {
		fd, err = osOpenFile(os.DevNull, os.O_WRONLY, 0)
		if err != nil {
			return
		}
		c.closeAfterStart = append(c.closeAfterStart, fd)
		return
	}

	// If the writer is already a file descriptor then just cast it.
	var ok bool
	if fd, ok = writer.(*os.File); ok {
		return
	}

	// The writer is a io.Writer but is not an os.File so a goroutine needs to
	// be started in order to shuttle data from the internal writer to the
	// real file descriptor.
	r, fd, err := osPipe()
	if err != nil {
		return
	}

	// Add the file descriptors to the various tracking routines.
	c.closeAfterStart = append(c.closeAfterStart, fd)
	c.closeAfterWait = append(c.closeAfterWait, r)
	c.errWG.Add(1)
	go func() {
		_, err := io.Copy(writer, r)
		c.setError(err)
		c.setError(r.Close())
		c.errWG.Done()
	}()
	return
}
