/*
 * sync_file_range.c - provide the sync_file_range IPC system call
 * since bionic doesn't supply this function
 */

#define _GNU_SOURCE
#include <sys/types.h>
#include <sys/syscall.h>
#include <sys/glibc-syscalls.h>
#include <unistd.h>
#include "android_compat.h"

int sync_file_range(int fd, off64_t offset, off64_t nbytes,
		    unsigned int flags)
{
	return syscall(SYS_sync_file_range, fd, offset, nbytes, flags);
}
