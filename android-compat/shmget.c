/*
 * shmget.c - provide the System V IPC system calls since bionic
 * doesn't supply these functions
 */

#define _GNU_SOURCE
#include <sys/types.h>
#include <sys/syscall.h>
#include <sys/glibc-syscalls.h>
#include <unistd.h>
#include "android_compat.h"

int shmctl(int shmid, int cmd, struct shmid_ds *buf)
{
	return syscall(SYS_shmctl, shmid, cmd, buf);
}

void *shmat(int shmid, const void *shmaddr, int shmflg)
{
	syscall(SYS_shmat, shmid, shmaddr, shmflg);
}

int shmdt(const void *shmaddr)
{
	return syscall(SYS_shmdt, shmaddr);
}

int shmget(int key, size_t size, int shmflg)
{
	return syscall(SYS_shmget, key, size, shmflg);
}
