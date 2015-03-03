# coremem

A utility to report core memory usage for a program, inspired
by @pixelb's [ps\_mem](https://github.com/pixelb/ps_mem)

Usage:

```
$ ./coremem
  Private  (  Shared)    RAM (PSS)	Program

200.0 KiB  ( 12.0 KiB)  212.0 KiB	dbus-launch
232.0 KiB  ( 10.0 KiB)  242.0 KiB	VidyoDesktop
320.0 KiB  (  6.0 KiB)  326.0 KiB	less
[...]
 55.3 MiB  (  3.9 MiB)   59.2 MiB	emacs-24.3-nox (3)
112.3 MiB  (  1.0 MiB)  113.3 MiB	bash (61)
155.6 MiB  (  3.9 MiB)  159.5 MiB	gnome-shell
168.6 MiB  ( 24.0 KiB)  168.6 MiB	crash
198.4 MiB  (  5.0 KiB)  198.4 MiB	git
256.5 MiB  (489.0 KiB)  257.0 MiB	tmux (8)
---------------------------------
               Pss total: 1.2 GiB
=================================
```

## Build:
    go build coremem.go

## Improvements over pixelb's python script:
1) written with Go language's concurrent model that makes it run much faster,
   on a server with 1400 processes running this takes 2s to print results,
   vs. the ps\_mem.py takes 36s;
   Go compiler's default output is a static binary, makes it useful with
   hosts where there is no python.

2) ignoring access permission error if run with normal user id, this is useful
   for desktop users, they can get core mem information for their own processes,
   while administrator can still run with sudo for the whole system.

## ToDo List
- [x] accurate Pss
- [ ] support specified pids
- [ ] request for comments
