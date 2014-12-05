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
	"fmt"
	"syscall"
	"testing"
)

func TestMmapDataAlloc(t *testing.T) {
	// Test 1: Mmap fails.
	func() {
		// Replace the syscallMmap pointer with a function that always errors.
		defer func() { syscallMmap = syscall.Mmap }()
		syscallMmap = func(fd int, off int64, l, p, f int) ([]byte, error) {
			return nil, fmt.Errorf("expected error")
		}

		data, err := mmapDataAlloc(1000)
		if err == nil {
			t.Fatalf("Expected error not returned.")
		} else if data != nil {
			t.Fatalf("data should have been nil.")
		}
	}()

	// Test 2: Success.
	func() {
		data, err := mmapDataAlloc(1)
		if err != nil {
			t.Fatalf("Unexpected mmap error: %s", err)
		} else if data == nil {
			t.Fatalf("data should not have been nil.")
		} else if len(data.data) != 1 {
			t.Fatalf("len(data.data) is %d, which is not 1.", len(data.data))
		}
	}()
}

func TestMmapData_PanicIfOverAllocated(t *testing.T) {
	// Passes
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Unexpected panic: %#v", r)
			}
		}()
		m := mmapData{
			data:   make([]byte, 1000),
			offset: 999,
		}
		m.panicIfOverAllocated()
	}()

	// Fails
	func() {
		defer func() {
			if r := recover(); r == nil {
				t.Fatalf("Expected panic not returned.")
			}
		}()
		m := mmapData{
			data:   make([]byte, 1000),
			offset: 1001,
		}
		m.panicIfOverAllocated()
	}()
}
