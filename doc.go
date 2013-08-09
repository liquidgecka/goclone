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

// This package provides a simple wrapper around the linux 'clone' system call.
// The aim was to be as compatible with os/exec as possible so this could be
// used as a direct drop in replacement. Some functionality is improved
// upon, but mostly this enables several things that can only be done with
// a wrapper around clone and some added cgo code.
//
// NAME SPACES
//
// This package supports either joining the name spaces of existing processes,
// or creating whole new name spaces for the process being executed. The
// current list of supported name spaces include IPC, Mount, Network, PID, and
// UTS, with PID being special in that its impossible to join a PID name space.
//
// Each of these will isolate the process in some way, for example, removing
// visibility of the network interfaces on a machine, hiding away other
// processes, making a whole new file system layout that does not impact
// other running processes, etc.
//
// For more information see this: http://lwn.net/Articles/531114/
//
// DOUBLE FORKING
//
// This package enables a golang binary to execute another binary by first
// double forking. This allows the new binary to be a child of initd (pid 1)
// rather than the calling process. This is very useful for making daemons.
//
// CGROUPS
//
// This package enables a process to be joined into cgroups before it is
// executed. This ensures that a process can not fork prior to being added
// or spawn new threads, etc.
//
// OTHER FEATURES
//
// These are features that exist in os/exec as well as this module.
//
// This package also supports changing user credentials via the SysProcAttr
// field, chrooting the process, writing to non os.File buffers via an
// automatically created goroutine.
//
// TODO
//
// There are still many things to do within this module:
// * Examples!
// * Implement the functionality in SysProcAttr fully.
// * Finish testing some of the more edge case functionality.
// * Better documentation.
package goclone
