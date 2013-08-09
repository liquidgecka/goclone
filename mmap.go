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

// #include "goclone.h"
import "C"

import (
	"syscall"
	"unsafe"
)

var (
	// The size of a pointer in bytes in the system.
	ptrSize = int(unsafe.Sizeof(unsafe.Pointer(nil)))

	// The size of a C integer in bytes on the system.
	intSize = int(unsafe.Sizeof(C.int(0)))

	// The size of a gid_t type in bytes on the system.
	gidSize = int(unsafe.Sizeof(C.gid_t(0)))

	// The size of system pages on the system.
	pageSize = syscall.Getpagesize()

	// The size that should be allocated for stacks.
	stackSize = pageSize * 8
)

// This function call will calculate exactly how much space is expected to be
// used by the data elements of the c array. The returned size here will
// not be page size shifted.
func rawDataSize(c *Cmd) int {
	// First calculate the size of the C.goclone_cmd, then add each of the
	// strings, or arrays to the size so we know how much space to allocate.
	dataSize := int(unsafe.Sizeof(C.goclone_cmd{}))
	dataSize += len(c.Path) + 1
	dataSize += (len(c.Args) + 1) * ptrSize
	for _, s := range c.Args {
		dataSize += len(s) + 1
	}
	dataSize += (len(c.Env) + 1) * ptrSize
	for _, s := range c.Env {
		dataSize += len(s) + 1
	}
	dataSize += len(c.Dir) + 1
	if c.SysProcAttr != nil {
		dataSize += len(c.SysProcAttr.Chroot) + 1
	} else {
		dataSize += 1
	}
	dataSize += (len(c.ExtraFiles) + 3) * intSize
	if c.SysProcAttr != nil && c.SysProcAttr.Credential != nil {
		dataSize += len(c.SysProcAttr.Credential.Groups) * gidSize
	}
	dataSize += (len(c.CgroupsTasksFiles) + 1) * ptrSize
	for _, s := range c.CgroupsTasksFiles {
		dataSize += len(s) + 1
	}
	dataSize += len(c.IPCNameSpace) + 1
	dataSize += len(c.MountNameSpace) + 1
	dataSize += len(c.NetworkNameSpace) + 1
	dataSize += len(c.UTSNameSpace) + 1

	return dataSize
}

// This object is manages a chunk of memory allocated via a call to mmap. It
// allows simple string copying, NULL terminated string copying, etc. This
// makes it far easier to track the data since it can't be done via normal
// allocations.
type mmapData struct {
	data   []byte
	offset int
}

// Called to allocate the given space using mmap.
func mmapDataAlloc(size int) (*mmapData, error) {
	flags := syscall.MAP_PRIVATE | syscall.MAP_ANONYMOUS
	prot := syscall.PROT_WRITE | syscall.PROT_READ
	data, err := syscallMmap(-1, 0, size, prot, flags)
	if err != nil {
		return nil, err
	}
	return &mmapData{data: data}, nil
}

// Unmaps the data in the mmapData structure.
func (m *mmapData) free() error {
	return syscallMunmap(m.data)
}

// Marks the next page in the mmap allocation as protected. This will prevent
// reads and writes to the data segment which is used to ensure that the
// process does not overflow its stack.
func (m *mmapData) mprotect() error {
	// Calculate the offset since it needs to be page aligned.
	m.offset = ((m.offset - 1) | (pageSize - 1)) + 1
	protected := m.data[m.offset : m.offset+pageSize]
	err := syscallMprotect(protected, syscall.PROT_NONE)
	m.offset += pageSize
	if m.offset > len(m.data) {
		panic("data over allocation.")
	}
	return err
}

// Allocates a chunk of ram in the buffer to be used as a stack. The pointer
// returned is for the last address in the stack space since stacks grow
// down from the top.
func (m *mmapData) stack() unsafe.Pointer {
	m.offset += stackSize
	return unsafe.Pointer(&m.data[m.offset-1])
}

// Adds a C.goclone_cmd to the buffer and returns a pointer to it.
func (m *mmapData) pushGocloneCmd() *C.goclone_cmd {
	ptr := (*C.goclone_cmd)(unsafe.Pointer(&m.data[m.offset]))
	m.offset += int(unsafe.Sizeof(C.goclone_cmd{}))
	if m.offset > len(m.data) {
		panic("data over allocation.")
	}
	return ptr
}

// Pushes an integer array into the mmapData buffer.
func (m *mmapData) pushIntSlice(is []int) *C.int{
	ptr := unsafe.Pointer(&m.data[m.offset])
	nextptr := ptr
	for _, i := range is {
		*(*C.int)(nextptr) = C.int(i)
		m.offset += intSize
		nextptr = unsafe.Pointer(&m.data[m.offset])
	}
	if m.offset > len(m.data) {
		panic("data over allocation.")
	}
	return (*C.int)(ptr)
}

// Pushes an array of gid_t's into the mmapData buffer.
func (m *mmapData) pushGidSlice(gs []uint32) *C.gid_t{
	ptr := unsafe.Pointer(&m.data[m.offset])
	nextptr := ptr
	for _, g := range gs {
		*(*C.gid_t)(nextptr) = C.gid_t(g)
		m.offset += gidSize
		nextptr = unsafe.Pointer(&m.data[m.offset])
	}
	if m.offset > len(m.data) {
		panic("data over allocation.")
	}
	return (*C.gid_t)(ptr)
}

// Pushes a string into the mmapData buffer, null terminates it, then returns
// a pointer to it.
func (m *mmapData) pushString(s string) *C.char {
	ptr := unsafe.Pointer(&m.data[m.offset])
	copy(m.data[m.offset:m.offset+len(s)], s)
	m.data[m.offset+len(s)] = 0
	m.offset += len(s) + 1
	if m.offset > len(m.data) {
		panic("data over allocation.")
	}
	return (*C.char)(ptr)
}

// Pushes a string slice into the mmapData buffer, null terminating the last
// element in the C array.
func (m *mmapData) pushStringSlice(ss []string) **C.char {
	arr := unsafe.Pointer(&m.data[m.offset])
	nextptr := arr
	m.offset += ptrSize * (len(ss) + 1)
	for _, s := range ss {
		ptr := m.pushString(s)
		*(*unsafe.Pointer)(nextptr) = unsafe.Pointer(ptr)
		nextptr = unsafe.Pointer(uintptr(nextptr) + uintptr(ptrSize))
	}
	*(*unsafe.Pointer)(nextptr) = unsafe.Pointer(nil)
	if m.offset > len(m.data) {
		panic("data over allocation.")
	}
	return (**C.char)(arr)
}
