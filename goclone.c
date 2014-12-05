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

#ifdef __linux

// Necessary to get clone() to appear via includes.
#define _GNU_SOURCE

// The maximum length of a filename read from the proc.
#define FILENAMESIZE 4096

// FIXME
#include <sys/mount.h>

#include <dirent.h>
#include <errno.h>
#include <fcntl.h>
#include <pthread.h>
#include <sched.h>
#include <stdio.h>
#include <string.h>
#include <sysexits.h>
#include <unistd.h>

#include "goclone.h"

// Set a boolean that tell us if user namespaces are enabled in the kernel.
#ifdef CLONE_NEWUSER
int goclone_user_namespace_enabled = 1;
#else
int goclone_user_namespace_enabled = 0;
#endif

// Force close a file descriptor.
static void close_fd(int fd)
{
    while (close(fd) == -1) {
        if (errno == EBADF) {
            // The file descriptor wasn't open.
            return;
        } else if (errno == EINTR) {
            // Interrupted. Try again.
            continue;
        } else {
            // Unknown error?
            _exit(EX_OSERR);
        }
    }
}

// This function is called in order to setup the file descriptors in the
// processes. This will deal with stdin, stdout, stderr as well as any
// of the extra files defined in the Cmd structure. Once all the file
// descriptors have been duplicated into place all other open file descriptors
// will be closed.
static void setup_files(goclone_cmd *cmd)
{
    DIR *d;
    char buffer[sizeof(struct dirent) + FILENAMESIZE];
    int closed;
    int fd;
    int i;
    struct dirent *results;

    // Walk through all of the file descriptors to see if any of the
    // descriptors are actually within the range that will be duped. If so
    // we need to copy the file descriptor out of the way.
    for (i = 0; i < cmd->files_len; i++) {
        if (cmd->files[i] == -1) {
            continue;
        } else if (cmd->files[i] >= cmd->files_len) {
            continue;
        } else if (cmd->files[i] == i) {
            continue;
        }

        // Its possible for a duped descriptor to land in the range we are
        // managing. If this happens then we need to dupe again until the
        // resulting descriptor is greater than the list.
        do {
            fd = dup(cmd->files[i]);
            if (fd == -1) {
                // FIXME: Log?
                _exit(EX_OSERR);
            }
        } while (fd < cmd->files_len);
        cmd->files[i] = fd;
    }

    // Now walk through duping all of the file descriptors.
    for (i = 0; i < cmd->files_len; i++) {
        if (cmd->files[i] == -1) {
            close_fd(i);
        } else {
            if (dup2(cmd->files[i], i) == -1) {
                // FIXME
                _exit(EX_OSERR);
            }
        }
    }

    // Now walk through closing all files larger than the list of descriptors
    // passed in. This ensures that all descriptors that are not desired are
    // closed, regardless of the "close on exec" status of the descriptor.
    do {
        closed = 0;

        // Open the /proc directory. This can only real fail if /proc is not
        // mounted.
        d = opendir("/proc/self/fdinfo");
        if (d == NULL) {
            _exit(EX_OSERR);
        }

        while (1) {
            // Read each element from the directory listing one at a time.
            if (readdir_r(d, (struct dirent *) &buffer, &results) != 0) {
                // FIXME: logging?
                _exit(EX_OSERR);
            }

            // NULL here is the end of the directory list.
            if (results == NULL) {
                break;
            }

            // Parse the file name into an integer.
            i = atoi(results->d_name);

            // Make sure that no descriptor less than the list given above
            // is not closed, and that the descriptor with the directory
            // listing is left open as well.
            if (i < cmd->files_len || i == dirfd(d)) {
                continue;
            }

            // Close the file descriptor.
            close_fd(i);
            closed++;
        }

        // Close the open directory file descriptor.
        if (closedir(d) == -1) {
            _exit(EX_OSERR);
        }

        // Keep looping while descriptors are being closed.
    } while (closed != 0);
}

// Joins any cgroup listed in the CgroupsTasksFiles
static void join_cgroups(goclone_cmd *cmd)
{
    ;
    // 20 is the maximum number of characters in an integer value.
    char buffer[20];
    int fd;
    int i;
    int len;

    len = sprintf(buffer, "%d\n", getpid());
    for (i = 0; cmd->cgroups_tasks_files[i] != NULL; i++) {
        fd = open(cmd->cgroups_tasks_files[i], O_APPEND | O_WRONLY);
        if (fd == -1) {
            _exit(EX_OSERR);
        }
        if (write(fd, buffer, len) != len) {
            _exit(EX_OSERR);
        }
        if (close(fd) == -1) {
            _exit(EX_OSERR);
        }
    }
}

// Joins a namespace listed in one of the name space fields.
static void join_namespace(char *namespace)
{
    int fd;

    if (namespace == NULL || namespace[0] == '\0') {
        return;
    }
    fd = open(namespace, O_RDONLY);
    if (fd == -1) {
        _exit(EX_OSERR);
    }
    if (setns(fd, 0) != 0) {
        _exit(EX_OSERR);
    }
    if (close(fd) != 0) {
        _exit(EX_OSERR);
    }
}

// Sets up the directories for this process.
static void setup_directories(goclone_cmd *cmd)
{
    // Chroot
    if (cmd->chroot_dir != NULL && cmd->chroot_dir[0] != '\0') {
        if (chroot(cmd->chroot_dir) == -1) {
            _exit(EX_OSERR);
        }
    }

    // Chdir
    if (cmd->dir != NULL && cmd->dir[0] != '\0') {
        if (chdir(cmd->dir) == -1) {
            _exit(EX_OSERR);
        }
    } else {
        if (chdir("/") == -1) {
            _exit(EX_OSERR);
        }
    }
}

// Mounts /proc if requested.
static void mount_proc(goclone_cmd *cmd)
{
    if (cmd->mount_new_proc != true) {
        return;
    }

    if (mount("none", "/proc", "proc", 0, NULL) != 0) {
        _exit(EX_OSERR);
    }
}

static void bindnode(char *src, char *dst)
{
    if (mount(src, dst, NULL, MS_BIND, NULL) < 0) {
        error(1, 0, "Failed to bind %s into new /dev filesystem", src);
    }
}

static void create_pseudo_devices(goclone_cmd *cmd)
{
    if (cmd->create_pseudo_devices != true) {
        return;
    }

    bindnode("/dev/full", "dev/full");
    bindnode("/dev/null", "dev/null");
    bindnode("/dev/random", "dev/random");
    bindnode("/dev/tty", "dev/tty");
    bindnode("/dev/urandom", "dev/urandom");
    bindnode("/dev/zero", "dev/zero");
}

// Sets up the credentials of the process if necessary.
static void set_credentials(goclone_cmd *cmd)
{
    if (cmd->set_credentials != true) {
        return;
    }

    // Set the group id first.
    if (setregid(cmd->gid, cmd->gid) != 0) {
        _exit(EX_OSERR);
    }
    if (getgid() != cmd->gid) {
        _exit(EX_OSERR);
    }

    // Set the list of static groups.
    if (setgroups(cmd->groups_len, cmd->groups) != 0) {
        _exit(EX_OSERR);
    }

    // Set the uid now that the group is in place.
    if (setreuid(cmd->uid, cmd->uid) != 0) {
        _exit(EX_OSERR);
    }
    if (getuid() != cmd->uid) {
        _exit(EX_OSERR);
    }
}

// clone() destination which will exec the real user process.
static int exec_func(void *v)
{
    goclone_cmd *cmd;
    cmd = (goclone_cmd *) v;

    // Setup the file descriptors.
    setup_files(cmd);

    // Join the various cgroups.
    join_cgroups(cmd);

    // Join the various name spaces.
    join_namespace(cmd->ipc_namespace);
    join_namespace(cmd->mount_namespace);
    join_namespace(cmd->network_namespace);
    join_namespace(cmd->pid_namespace);
    join_namespace(cmd->user_namespace);
    join_namespace(cmd->uts_namespace);

    // Directory management.
    setup_directories(cmd);

    // Mount /proc
    mount_proc(cmd);

    // Create pseudo devices: tty, zero, null, full, random, urandom
    create_pseudo_devices(cmd);

    // Set credentials.
    set_credentials(cmd);

    // Attempt to lock the mutex. Until this mutex can be locked we need
    // to block so the parent can complete any work necessary.
    pthread_mutex_lock(&(cmd->pre_exec_mutex));

    // Execute the command.
    execve(cmd->path, cmd->args, cmd->env);

    // This can not be reached unless something has gone wrong.
    _exit(EX_OSERR);
}

// This function generates the flags that should be used for the clone
// that calls exec_func.
static int clone_flags(goclone_cmd *cmd)
{
    int flags;

    flags = 0 | cmd->death_signal;
    if (cmd->new_ipc_namespace) {
        flags |= CLONE_NEWIPC;
    }
    if (cmd->new_network_namespace) {
        flags |= CLONE_NEWNET;
    }
    if (cmd->new_mount_namespace) {
        flags |= CLONE_NEWNS;
    }
    if (cmd->new_pid_namespace) {
        flags |= CLONE_NEWPID;
    }
    if (cmd->new_user_namespace) {
        flags |= CLONE_NEWUSER;
    }
    if (cmd->new_uts_namespace) {
        flags |= CLONE_NEWUTS;
    }
    return flags;
}

// This function is called if double forking is requested. It is the
// intermediate child that will terminate once the execve is run.
static int df_func(void *v)
{
    goclone_cmd *cmd;
    int flags;

    cmd = (goclone_cmd *) v;
    flags = clone_flags(cmd);
    if (clone(exec_func, cmd->stack, flags, cmd) == -1) {
        _exit(EX_OSERR);
    }
    _exit(0);
}

// Writes the given values completely to the given file.
static int write_file(char *fn, char *data, int data_len)
{
    int fd;

    if ((fd = open(fn, O_WRONLY | O_TRUNC)) < 0) {
        return -1;
    }
    if (write(fd, data, data_len) != data_len) {
        if (close(fd) == -1) {
            return -1;
        }
        return -1;
    }
    if (close(fd) == -1) {
        return -1;
    }

    return 0;
}

// This function is used to write the contents of a map file.
static int write_mapfiles(goclone_parent_data *data, pid_t pid)
{
    int fd;
    char fn[FILENAMESIZE];

    // uid_map
    if (data->uid_map != NULL) {
        if (sprintf(fn, "/proc/%d/uid_map", (int)pid) < 0) {
            return -1;
        }
        if (write_file(fn, data->uid_map, data->uid_map_length) != 0) {
            return -1;
        }
    }

    // gid_map
    if (data->gid_map != NULL) {
        if (sprintf(fn, "/proc/%d/gid_map", (int)pid) < 0) {
            return -1;
        }
        if (write_file(fn, data->gid_map, data->gid_map_length) != 0) {
            return -1;
        }
    }

    // Success
    return 0;
}


// This is the destination function called from inside of Start().
pid_t goclone(goclone_cmd *cmd, goclone_parent_data *data)
{
    int flags;
    pid_t pid;

    // Initialize the mutex.
    pthread_mutexattr_init(&(cmd->pre_exec_mutex_attr));
    pthread_mutexattr_setpshared(
        &(cmd->pre_exec_mutex_attr), PTHREAD_PROCESS_SHARED);
    pthread_mutex_init(
        &(cmd->pre_exec_mutex), &(cmd->pre_exec_mutex_attr));
    pthread_mutex_lock(&(cmd->pre_exec_mutex));

    if (cmd->double_fork) {
        flags = 0 | cmd->death_signal;
        pid = clone(df_func, cmd->df_stack, flags, cmd);
    } else {
        flags = clone_flags(cmd);
        pid = clone(exec_func, cmd->stack, flags, cmd);
    }

    // Make sure that the uid and gid map files get written if they were
    // defined in the data structure.
    write_mapfiles(data, pid);

    // Unlock the mutex so that the child may continue.
    pthread_mutex_unlock(&(cmd->pre_exec_mutex));

    return pid;
}

#endif // __linux
