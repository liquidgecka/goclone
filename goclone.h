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

#ifndef GOCLONE_H
#define GOCLONE_H

#include <stdbool.h>
#include <sys/types.h>

// This structure is actually stored inside of data that is allocated via
// a call to memmap. This ensures that malloc/free is not used, and that the
// memory space is clean.
typedef struct goclone_cmd {
	// Exec settings.
	char *path;
	char **args;
	char **env;
	char *dir;
	char *chroot_dir;

	// File descriptors.
	int *files;
	int files_len;

	// Credentials
	bool set_credentials;
	uid_t uid;
	gid_t gid;
	gid_t *groups;
	int groups_len;

	// Cgroups tasks files.
	char **cgroups_tasks_files;

	// Namespaces that should be joined after clone.
	char *ipc_namespace;
	char *mount_namespace;
	char *network_namespace;
	char *uts_namespace;
	bool new_ipc_namespace;
	bool new_network_namespace;
	bool new_mount_namespace;
	bool new_pid_namespace;
	bool new_uts_namespace;

	// Set to true if the process should double fork.
	bool double_fork;

	// The signal that should be used when the parent dies.
	int death_signal;

	// The stack that should be used for the clone destination.
	void *stack;

	// If double forking is set then this stack should be set to a block
	// of memory allocated specifically for it.
	void *df_stack;

} goclone_cmd;

// This is the function that actually performs the underlying clone call. This
// should never be called directly by a user.
extern pid_t goclone(goclone_cmd *cmd);

#endif
