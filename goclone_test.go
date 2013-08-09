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
	"syscall"
	"testing"
)

const (
	catBin   = "/bin/cat"
	echoBin  = "/bin/echo"
	falseBin = "/bin/false"
	shBin    = "/bin/sh"
	sleepBin = "/bin/sleep"
	touchBin = "/usr/bin/touch"
	trueBin  = "/bin/true"
)

func TestNecessaryBinariesExist(t *testing.T) {
	if _, err := os.Lstat(catBin); err != nil {
		t.Errorf("Missing %s", catBin)
	} else if _, err = os.Lstat(echoBin); err != nil {
		t.Errorf("Missing %s", echoBin)
	} else if _, err = os.Lstat(falseBin); err != nil {
		t.Errorf("Missing %s", falseBin)
	} else if _, err = os.Lstat(shBin); err != nil {
		t.Errorf("Missing %s", shBin)
	} else if _, err = os.Lstat(sleepBin); err != nil {
		t.Errorf("Missing %s", sleepBin)
	} else if _, err = os.Lstat(touchBin); err != nil {
		t.Errorf("Missing %s", touchBin)
	} else if _, err = os.Lstat(trueBin); err != nil {
		t.Errorf("Missing %s", trueBin)
	}
}

func TestCommand(t *testing.T) {
	cmd := Command("path", "arg1", "arg2")
	if cmd.Path != "path" {
		t.Fatalf("cmd.Path not set properly: %s", cmd.Path)
	}
	if len(cmd.Args) != 3 {
		t.Fatalf("cmd.Args not 3 elements: %#v", cmd.Args)
	}
	if cmd.Args[0] != "path" {
		t.Fatalf("cmd.Args[0] not 'path': %#v", cmd.Args)
	}
	if cmd.Args[1] != "arg1" {
		t.Fatalf("cmd.Args[1] not 'arg1': %#v", cmd.Args)
	}
	if cmd.Args[2] != "arg2" {
		t.Fatalf("cmd.Args[2] not 'arg2': %#v", cmd.Args)
	}
}

func TestCmd_CombinedOutputSimple(t *testing.T) {
	command := fmt.Sprintf("%s 'stdout' && %s 'stderr' >&2", echoBin, echoBin)
	cmd := Command(shBin, "-c", command)
	data, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Unexpected error running command: %s", err)
	}
	if string(data) != "stdout\nstderr\n" {
		t.Fatalf("Unexpected output:\n%s", string(data))
	}
}

func TestCmd_CombinedOutputAlreadyStarted(t *testing.T) {
	cmd := Command(sleepBin, "1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Unexpected error starting command: %s", err)
	}
	if _, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("Expected error not returned.")
	} else if err.Error() != "goclone: Process already started." {
		t.Fatalf("Wrong error returned: %s", err)
	}
	cmd.Process.Kill()
	cmd.Wait()
}

func TestCmd_CombinedOutputStdoutAlreadyDefined(t *testing.T) {
	cmd := Command(sleepBin, "1")
	cmd.Stdout = os.Stdout
	if _, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("Expected error not returned.")
	} else if err.Error() != "goclone: Stdout already set." {
		t.Fatalf("Wrong error returned: %s", err)
	}
}

func TestCmd_CombinedOutputStderrAlreadyDefined(t *testing.T) {
	cmd := Command(sleepBin, "1")
	cmd.Stderr = os.Stderr
	if _, err := cmd.CombinedOutput(); err == nil {
		t.Fatalf("Expected error not returned.")
	} else if err.Error() != "goclone: Stderr already set." {
		t.Fatalf("Wrong error returned: %s", err)
	}
}

func TestCmd_OutputSimple(t *testing.T) {
	command := fmt.Sprintf("%s 'stdout' && %s 'stderr' >&2", echoBin, echoBin)
	cmd := Command(shBin, "-c", command)
	data, err := cmd.Output()
	if err != nil {
		t.Fatalf("Unexpected error running command: %s", err)
	}
	if string(data) != "stdout\n" {
		t.Fatalf("Unexpected output:\n%s", string(data))
	}
}

func TestCmd_OutputAlreadyStarted(t *testing.T) {
	cmd := Command(sleepBin, "1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Unexpected error starting command: %s", err)
	}
	if _, err := cmd.Output(); err == nil {
		t.Fatalf("Expected error not returned.")
	} else if err.Error() != "goclone: Process already started." {
		t.Fatalf("Wrong error returned: %s", err)
	}
	cmd.Process.Kill()
	cmd.Wait()
}

func TestCmd_OutputStdoutAlreadyDefined(t *testing.T) {
	cmd := Command(sleepBin, "1")
	cmd.Stdout = os.Stdout
	if _, err := cmd.Output(); err == nil {
		t.Fatalf("Expected error not returned.")
	} else if err.Error() != "goclone: Stdout already set." {
		t.Fatalf("Wrong error returned: %s", err)
	}
}

func TestCmd_Run(t *testing.T) {
	tempfile, err := ioutil.TempFile("", "TestRun")
	if err != nil {
		t.Fatalf("Unexpected error: %s", err)
	} else if err := os.Remove(tempfile.Name()); err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	defer os.Remove(tempfile.Name())
	cmd := Command(shBin, "-c", fmt.Sprintf(
		"%s %s", touchBin, tempfile.Name()))
	if err := cmd.Run(); err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	if _, err := os.Lstat(tempfile.Name()); err != nil {
		if os.IsNotExist(err) {
			t.Fatalf("Command didn't run.")
		} else {
			t.Fatalf("Error running stat on temp file: %s", err)
		}
	}
}

func TestCmd_RunStartError(t *testing.T) {
	cmd := Command(sleepBin, "1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	if err := cmd.Run(); err == nil {
		t.Fatalf("Expected error not returned.")
	}
}

func TestCmd_RunWaitError(t *testing.T) {
	cmd := Command(falseBin)
	if err := cmd.Run(); err == nil {
		t.Fatalf("Expected error not returned.")
	} else if err.Error() != "exit status 1" {
		t.Fatalf("Wrong error returned: %s", err)
	}
}

func TestCmd_Start(t *testing.T) {
	// FIXME
}

func TestCmd_StartAlreadyRunning(t *testing.T) {
	cmd := Command(sleepBin, "1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	if err := cmd.Start(); err == nil {
		t.Fatalf("Expecter error not returned.")
	} else if err.Error() != "goclone: already started." {
		t.Fatalf("Wrong error returned: %s", err)
	}
}

func TestCmd_StartStdinError(t *testing.T) {
	// Replace the osOpen pointer with a function that always errors.
	defer func() { osOpen = os.Open }()
	osOpen = func(name string) (*os.File, error) {
		return nil, fmt.Errorf("expected error")
	}

	cmd := Command(trueBin)
	if err := cmd.Start(); err == nil {
		t.Fatalf("Expected error not returned.")
	} else if err.Error() != "expected error" {
		t.Fatalf("Wrong error returned: %s", err)
	}
}

func TestCmd_StartStdoutError(t *testing.T) {
	// Replace the osOpeniFile pointer with a function that always errors.
	defer func() { osOpenFile = os.OpenFile }()
	osOpenFile = func(n string, f int, m os.FileMode) (*os.File, error) {
		return nil, fmt.Errorf("expected error")
	}

	cmd := Command(trueBin)
	if err := cmd.Start(); err == nil {
		t.Fatalf("Expected error not returned.")
	} else if err.Error() != "expected error" {
		t.Fatalf("Wrong error returned: %s", err)
	}
}

func TestCmd_StartStderrError(t *testing.T) {
	// Replace the osOpeniFile pointer with a function that always errors.
	defer func() { osOpenFile = os.OpenFile }()
	osOpenFile = func(n string, f int, m os.FileMode) (*os.File, error) {
		return nil, fmt.Errorf("expected error")
	}

	cmd := Command(trueBin)
	cmd.Stdout = &bytes.Buffer{}
	if err := cmd.Start(); err == nil {
		t.Fatalf("Expected error not returned.")
	} else if err.Error() != "expected error" {
		t.Fatalf("Wrong error returned: %s", err)
	}
}

func TestCmd_StartMmapError(t *testing.T) {
	// Replace the syscallMmap pointer with a function that always errors.
	defer func() { syscallMmap = syscall.Mmap }()
	syscallMmap = func(fd int, off int64, l, p, f int) ([]byte, error) {
		return nil, fmt.Errorf("expected error")
	}

	cmd := Command(trueBin)
	if err := cmd.Start(); err == nil {
		t.Fatalf("Expected error not returned.")
	} else if err.Error() != "expected error" {
		t.Fatalf("Wrong error returned: %s", err)
	}
}

func TestCmd_StartFirstMprotectError(t *testing.T) {
	// Replace the syscallMprotect pointer with a function that always errors.
	defer func() { syscallMprotect = syscall.Mprotect }()
	syscallMprotect = func(data []byte, i int) error {
		return fmt.Errorf("expected error")
	}

	// Make sure that Start() fails.
	cmd := Command(trueBin)
	if err := cmd.Start(); err == nil {
		t.Fatalf("Expected error not returned.")
	} else if err.Error() != "expected error" {
		t.Fatalf("Wrong error returned: %s", err)
	}
}

func TestCmd_StartSecondMprotectError(t *testing.T) {
	// Replace the syscallMprotect pointer with a function that always errors.
	count := 0
	defer func() { syscallMprotect = syscall.Mprotect }()
	syscallMprotect = func(data []byte, i int) error {
		count++
		if count == 2 {
			return fmt.Errorf("expected error")
		} else {
			return syscall.Mprotect(data, i)
		}
	}

	cmd := Command(trueBin)
	if err := cmd.Start(); err == nil {
		t.Fatalf("Expected error not returned.")
	} else if err.Error() != "expected error" {
		t.Fatalf("Wrong error returned: %s", err)
	}
}

func TestCmd_StartThirdMprotectError(t *testing.T) {
	// Replace the syscallMprotect pointer with a function that always errors.
	count := 0
	defer func() { syscallMprotect = syscall.Mprotect }()
	syscallMprotect = func(data []byte, i int) error {
		count++
		if count == 3 {
			return fmt.Errorf("expected error")
		} else {
			return syscall.Mprotect(data, i)
		}
	}

	cmd := Command(trueBin)
	cmd.DoubleFork = true
	if err := cmd.Start(); err == nil {
		t.Fatalf("Expected error not returned.")
	} else if err.Error() != "expected error" {
		t.Fatalf("Wrong error returned: %s", err)
	}
}

func TestCmd_StartMunmapError(t *testing.T) {
	// Replace the syscallMunmap pointer with a function that always errors.
	defer func() { syscallMunmap = syscall.Munmap }()
	syscallMunmap = func(data []byte) error {
		if err := syscall.Munmap(data); err != nil {
			t.Fatalf("Unexpected error: %s", err)
		}
		return fmt.Errorf("expected error")
	}

	cmd := Command(trueBin)
	if err := cmd.Start(); err == nil {
		t.Fatalf("Expected error not returned.")
	} else if err.Error() != "expected error" {
		t.Fatalf("Wrong error returned: %s", err)
	}
}

func TestCmd_StderrPipe(t *testing.T) {
	cmd := Command(sleepBin, "1")
	fd, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("Unexpected error returned: %s", err)
	}
	for _, w := range cmd.closeAfterWait {
		if w != fd {
			t.Fatalf("Unexpected file descriptor found: %s", w)
		}
	}
	if len(cmd.closeAfterWait) != 1 {
		t.Fatalf("Too many elements in the closeAfterWait list.")
	} else if len(cmd.closeAfterStart) != 1 {
		t.Fatalf("Too many elements in the closeAfterStart list.")
	}
}

func TestCmd_StderrPipeAlreadyStarted(t *testing.T) {
	cmd := Command(sleepBin, "1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Unexpected error starting command: %s", err)
	}
	if _, err := cmd.StderrPipe(); err == nil {
		t.Fatalf("Expected error not returned.")
	} else if err.Error() != "goclone: Process already started." {
		t.Fatalf("Wrong error returned: %s", err)
	}
	cmd.Process.Kill()
	cmd.Wait()
}

func TestCmd_StderrPipeStderrAlreadySet(t *testing.T) {
	cmd := Command(sleepBin, "1")
	cmd.Stderr = &bytes.Buffer{}
	if _, err := cmd.StderrPipe(); err == nil {
		t.Fatalf("Expected error not returned.")
	} else if err.Error() != "goclone: Stderr already set" {
		t.Fatalf("Wrong error returned: %s", err)
	}
}

func TestCmd_StderrPipeOsPipeError(t *testing.T) {
	// Mock out the osPipe function.
	defer func() { osPipe = os.Pipe }()
	osPipe = func() (*os.File, *os.File, error) {
		return nil, nil, fmt.Errorf("Expected error.")
	}

	cmd := Command(sleepBin, "1")
	if _, err := cmd.StderrPipe(); err == nil {
		t.Fatalf("Expected error not returned.")
	} else if err.Error() != "Expected error." {
		t.Fatalf("Wrong error returned: %s", err)
	}
}

func TestCmd_StdinPipe(t *testing.T) {
	cmd := Command(sleepBin, "1")
	fd, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("Unexpected error returned: %s", err)
	}
	for _, w := range cmd.closeAfterWait {
		if w != fd {
			t.Fatalf("Unexpected file descriptor found: %s", w)
		}
	}
	if len(cmd.closeAfterWait) != 1 {
		t.Fatalf("Too many elements in the closeAfterWait list.")
	} else if len(cmd.closeAfterStart) != 1 {
		t.Fatalf("Too many elements in the closeAfterStart list.")
	}
}

func TestCmd_StdinPipeAlreadyStarted(t *testing.T) {
	cmd := Command(sleepBin, "1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Unexpected error starting command: %s", err)
	}
	if _, err := cmd.StdinPipe(); err == nil {
		t.Fatalf("Expected error not returned.")
	} else if err.Error() != "goclone: Process already started." {
		t.Fatalf("Wrong error returned: %s", err)
	}
	cmd.Process.Kill()
	cmd.Wait()
}

func TestCmd_StdinPipeStdinAlreadySet(t *testing.T) {
	cmd := Command(sleepBin, "1")
	cmd.Stdin = &bytes.Buffer{}
	if _, err := cmd.StdinPipe(); err == nil {
		t.Fatalf("Expected error not returned.")
	} else if err.Error() != "goclone: Stdin already set" {
		t.Fatalf("Wrong error returned: %s", err)
	}
}

func TestCmd_StdinPipeOsPipeError(t *testing.T) {
	// Mock out the osPipe function.
	defer func() { osPipe = os.Pipe }()
	osPipe = func() (*os.File, *os.File, error) {
		return nil, nil, fmt.Errorf("Expected error.")
	}

	cmd := Command(sleepBin, "1")
	if _, err := cmd.StdinPipe(); err == nil {
		t.Fatalf("Expected error not returned.")
	} else if err.Error() != "Expected error." {
		t.Fatalf("Wrong error returned: %s", err)
	}
}

func TestCmd_StdoutPipe(t *testing.T) {
	cmd := Command(sleepBin, "1")
	fd, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("Unexpected error returned: %s", err)
	}
	for _, w := range cmd.closeAfterWait {
		if w != fd {
			t.Fatalf("Unexpected file descriptor found: %s", w)
		}
	}
	if len(cmd.closeAfterWait) != 1 {
		t.Fatalf("Too many elements in the closeAfterWait list.")
	} else if len(cmd.closeAfterStart) != 1 {
		t.Fatalf("Too many elements in the closeAfterStart list.")
	}
}

func TestCmd_StdoutPipeAlreadyStarted(t *testing.T) {
	cmd := Command(sleepBin, "1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("Unexpected error starting command: %s", err)
	}
	if _, err := cmd.StdoutPipe(); err == nil {
		t.Fatalf("Expected error not returned.")
	} else if err.Error() != "goclone: Process already started." {
		t.Fatalf("Wrong error returned: %s", err)
	}
	cmd.Process.Kill()
	cmd.Wait()
}

func TestCmd_StdoutPipeStdoutAlreadySet(t *testing.T) {
	cmd := Command(sleepBin, "1")
	cmd.Stdout = &bytes.Buffer{}
	if _, err := cmd.StdoutPipe(); err == nil {
		t.Fatalf("Expected error not returned.")
	} else if err.Error() != "goclone: Stdout already set" {
		t.Fatalf("Wrong error returned: %s", err)
	}
}

func TestCmd_StdoutPipeOsPipeError(t *testing.T) {
	// Mock out the osPipe function.
	defer func() { osPipe = os.Pipe }()
	osPipe = func() (*os.File, *os.File, error) {
		return nil, nil, fmt.Errorf("Expected error.")
	}

	cmd := Command(sleepBin, "1")
	if _, err := cmd.StdoutPipe(); err == nil {
		t.Fatalf("Expected error not returned.")
	} else if err.Error() != "Expected error." {
		t.Fatalf("Wrong error returned: %s", err)
	}
}

func TestCmd_Wait(t *testing.T) {
	cmd := Command(trueBin)
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &bytes.Buffer{}
	if err := cmd.Start(); err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
}

func TestCmd_WaitNotStarted(t *testing.T) {
	cmd := Command(sleepBin, "1")
	if err := cmd.Wait(); err == nil {
		t.Fatalf("Expected error not returned.")
	} else if err.Error() != "goclone: not started." {
		t.Fatalf("Wrong error returned: %s", err)
	}
}

func TestCmd_WaitAlreadyCalled(t *testing.T) {
	cmd := Command(trueBin)
	if err := cmd.Start(); err != nil {
		t.Fatalf("Unexpected error returned: %s", err)
	} else if err := cmd.Wait(); err != nil {
		t.Fatalf("Unexpected error returned: %s", err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatalf("Expected error not returned.")
	} else if err.Error() != "goclone: Wait was already called." {
		t.Fatalf("Wrong error returned: %s", err)
	}
}

func TestCmd_WaitProcessWaitError(t *testing.T) {
	cmd := Command(trueBin)
	// Make a fake Process structure so Wait errors out.
	cmd.Process, _ = os.FindProcess(-1)
	if err := cmd.Wait(); err == nil {
		t.Fatalf("Expected error not returned.")
	} else if err.Error() != "invalid argument" {
		t.Fatalf("Wrong error returned: %s", err)
	}
}

func TestCmd_WaitPreExistingError(t *testing.T) {
	cmd := Command(trueBin)
	if err := cmd.Start(); err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	cmd.setError(fmt.Errorf("Expected error."))
	if err := cmd.Wait(); err == nil {
		t.Fatalf("Expected error not returned.")
	} else if err.Error() != "Expected error." {
		t.Fatalf("Wrong error returned: %s", err)
	}
}

func TestCmd_WaitExitError(t *testing.T) {
	cmd := Command(falseBin)
	if err := cmd.Start(); err != nil {
		t.Fatalf("Unexpected error: %s", err)
	}
	if err := cmd.Wait(); err == nil {
		t.Fatalf("Expected error not returned.")
	} else if err.Error() != "exit status 1" {
		t.Fatalf("Wrong error returned: %s", err)
	}
}

func Test_closeDescriptors(t *testing.T) {
	openFile := func() *os.File {
		fd, err := os.Open(os.DevNull)
		if err != nil {
			t.Fatalf("Unexpected error: %s", err)
		}
		return fd
	}
	fds := []io.Closer{openFile(), openFile(), openFile()}
	err := fmt.Errorf("pass through error")
	if closeDescriptors(fds, err) != err {
		t.Fatalf("Err not passed through the call.")
	}

	// FIXME: check the fds?

	t.Skip("Test not finished.")
}

func TestCmd_reader(t *testing.T) {
	t.Skip("FIXME: Test not implemented.")
}

func TestCmd_setErrorFatalIsIgnored(t *testing.T) {
	c := Cmd{err: nil}
	c.setError(fmt.Errorf("error"))
	if c.err == nil {
		t.Fatalf("c.err should have been replaced.")
	}
}

func TestCmd_setError(t *testing.T) {
	c := Cmd{err: fmt.Errorf("not nil")}
	c.setError(nil)
	if c.err == nil {
		t.Fatalf("c.err was replaced with nil.")
	}
}

func TestCmd_setErrorNotReplacedIfAlreadySet(t *testing.T) {
	c := Cmd{err: fmt.Errorf("old")}
	c.setError(fmt.Errorf("new"))
	if c.err.Error() == "new" {
		t.Fatalf("c.err was replaced with a new value.")
	}
}

func TestCmd_writer(t *testing.T) {
	t.Skip("FIXME: Test not implemented.")
}
