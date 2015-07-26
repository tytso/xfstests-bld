/*
 * ustat.c - emulate ustat() since bionic doesn't supply this function
 */

#define _GNU_SOURCE
#include <sys/types.h>
#include <sys/syscall.h>
#include <sys/glibc-syscalls.h>
#include <unistd.h>
#include "ustat.h"

#ifndef __NR_ustat
#if defined(__aarch64__)
#define __NR_ustat 1070
#else
#error __NR_ustat not defined
#endif
#endif

int ustat(dev_t dev, struct ustat *ubuf)
{
	return syscall(SYS_ustat, dev, ubuf);
}
