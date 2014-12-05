goclone
=======

A go library that wraps the linux 'clone' system call.

Most of this modules documentation is found in its godoc which is accessible
at via the pkgdoc.org website: [godoc](http://go.pkgdoc.org/github.com/liquidgecka/goclone)

Primarily this package was intended to give access to linux name spaces in
a usable way for golang programs.

[![Continuous Integration](https://secure.travis-ci.org/liquidgecka/goclone.svg?branch=master)](http://travis-ci.org/liquidgecka/goclone)
[![Documentation](http://godoc.org/github.com/liquidgecka/goclone?status.png)](http://godoc.org/github.com/liquidgecka/goclone)
[![Coverage](https://img.shields.io/coveralls/liquidgecka/goclone.svg)](https://coveralls.io/r/liquidgecka/goclone)

Examples
========

Run command in new namespace

```Go
cmd := goclone.Command("/bin/bash")

// Set namespaces
cmd.NewIPCNameSpace = true
cmd.NewMountNameSpace = true
cmd.NewNetworkNameSpace = true
cmd.NewPIDNameSpace = true
cmd.NewUTSNameSpace = true
// Set work dir in namespace
cmd.Dir = "/"
// Set hostname
cmd.Hostname = "foo-bar.com"
// Create pseudo-devices: tty, zero, null, full, random, urandom
cmd.CreatePseudoDevices = true
// Enter to chroot
cmd.SysProcAttr = &syscall.SysProcAttr{
    Chroot: "/my_super_jail",
}
// Set uid, gid in chroot
uid, gid := 0, 0
cmd.SysProcAttr.Credential = &syscall.Credential{
    Uid: uint32(uid),
    Gid: uint32(gid),
}
// Redirect stdin, stdout, stderr
cmd.Stdin = os.Stdin
cmd.Stderr = os.Stderr
cmd.Stdout = os.Stdout
// Start command
if err := cmd.Start(); err != nil {
    log.Fatalf("Failed to start command: %s", err)
}
// Get pid
pid := cmd.Process.Pid
log.Println("Command started", cmd.Process.Pid)
// Run and wait command
if err := cmd.Wait(); err != nil {
    if _, ok := err.(*exec.ExitError); !ok {
        log.Fatalf("Failed to exit: %s", err)
    }
}
```

if need exec command in exists namespace:

```Go
var pid_i int

// ...

cmd := goclone.Command("/bin/bash")

// ...

pid := strconv.Itoa(pid_i)
cmd.IPCNameSpace = "/proc/" + pid + "/ns/ipc"
cmd.MountNameSpace = "/proc/" + pid + "/ns/mnt"
cmd.NetworkNameSpace = "/proc/" + pid + "/ns/net"
cmd.UTSNameSpace = "/proc/" + pid + "/ns/uts"
cmd.PIDNameSpace = "/proc/" + pid + "/ns/pid"

// ...
```
