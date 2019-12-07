/*
 * quotactl.c - emulate quotactl() since bionic doesn't supply this function
 */

#define _GNU_SOURCE
#include <sys/types.h>
#include <sys/syscall.h>
#include <unistd.h>
#include "quota.h"

#ifndef __NR_quotactl
#if defined(__aarch64__)
#define __NR_quotactl 60
#else
#error __NR_quotactl not defined
#endif
#endif

int quotactl(int cmd, const char *special, int id, caddr_t addr)
{
	return syscall(SYS_quotactl, cmd, special, id, addr);
}
