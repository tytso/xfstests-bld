/*
 * Copyright (c) 2001-2002 Silicon Graphics, Inc.
 * All Rights Reserved.
 *
 * This program is free software: you can redistribute it and/or modify it
 * under the terms of the GNU Lesser General Public License as published by
 * the Free Software Foundation, either version 2.1 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU Lesser General Public License for more details.
 *
 * You should have received a copy of the GNU Lesser General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 */

/*
 * The use of the syscall() function is an additional level of
 * indirection.  This avoids the dependency on kernel sources.
 */

#include <errno.h>
#include <unistd.h>
#include <sys/syscall.h>

#if defined (__NR_setxattr)
# define HAVE_XATTR_SYSCALLS 1
#else
# warning "Extended attribute syscalls undefined for this architecture"
# define HAVE_XATTR_SYSCALLS 0
#endif

#if HAVE_XATTR_SYSCALLS
# define SYSCALL(args...)	syscall(args)
#else
# define SYSCALL(args...)	( errno = ENOSYS, -1 )
#endif

int setxattr (const char *path, const char *name,
			void *value, size_t size, int flags)
{
	return SYSCALL(__NR_setxattr, path, name, value, size, flags);
}

int lsetxattr (const char *path, const char *name,
			void *value, size_t size, int flags)
{
	return SYSCALL(__NR_lsetxattr, path, name, value, size, flags);
}

int fsetxattr (int filedes, const char *name,
			void *value, size_t size, int flags)
{
	return SYSCALL(__NR_fsetxattr, filedes, name, value, size, flags);
}

ssize_t getxattr (const char *path, const char *name,
				void *value, size_t size)
{
	return SYSCALL(__NR_getxattr, path, name, value, size);
}

ssize_t lgetxattr (const char *path, const char *name,
				void *value, size_t size)
{
	return SYSCALL(__NR_lgetxattr, path, name, value, size);
}

ssize_t fgetxattr (int filedes, const char *name,
				void *value, size_t size)
{
	return SYSCALL(__NR_fgetxattr, filedes, name, value, size);
}

ssize_t listxattr (const char *path, char *list, size_t size)
{
	return SYSCALL(__NR_listxattr, path, list, size);
}

ssize_t llistxattr (const char *path, char *list, size_t size)
{
	return SYSCALL(__NR_llistxattr, path, list, size);
}

ssize_t flistxattr (int filedes, char *list, size_t size)
{
	return SYSCALL(__NR_flistxattr, filedes, list, size);
}

int removexattr (const char *path, const char *name)
{
	return SYSCALL(__NR_removexattr, path, name);
}

int lremovexattr (const char *path, const char *name)
{
	return SYSCALL(__NR_lremovexattr, path, name);
}

int fremovexattr (int filedes, const char *name)
{
	return SYSCALL(__NR_fremovexattr, filedes, name);
}
