/*
 * Copyright (c) 2001-2002 Silicon Graphics, Inc.
 * All Rights Reserved.
 *
 * This program is free software; you can redistribute it and/or
 * modify it under the terms of the GNU General Public License as
 * published by the Free Software Foundation.
 *
 * This program is distributed in the hope that it would be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program; if not, write the Free Software Foundation,
 * Inc.,  51 Franklin St, Fifth Floor, Boston, MA  02110-1301  USA
 */

/*
 * The use of the syscall() function is an additional level of
 * indirection.  This avoids the dependency on kernel sources.
 */

#include <errno.h>
#include <unistd.h>

#if defined (__i386__)
# define HAVE_XATTR_SYSCALLS 1
# define __NR_setxattr		226
# define __NR_lsetxattr		227
# define __NR_fsetxattr		228
# define __NR_getxattr		229
# define __NR_lgetxattr		230
# define __NR_fgetxattr		231
# define __NR_listxattr		232
# define __NR_llistxattr	233
# define __NR_flistxattr	234
# define __NR_removexattr	235
# define __NR_lremovexattr	236
# define __NR_fremovexattr	237
#elif defined (__sparc__)
# define HAVE_XATTR_SYSCALLS 1
# define __NR_setxattr		169
# define __NR_lsetxattr		170
# define __NR_fsetxattr		171
# define __NR_getxattr		172
# define __NR_lgetxattr		173
# define __NR_fgetxattr		177
# define __NR_listxattr		178
# define __NR_llistxattr	179
# define __NR_flistxattr	180
# define __NR_removexattr	181
# define __NR_lremovexattr	182
# define __NR_fremovexattr	186
#elif defined (__ia64__)
# define HAVE_XATTR_SYSCALLS 1
# define __NR_setxattr		1217
# define __NR_lsetxattr		1218
# define __NR_fsetxattr		1219
# define __NR_getxattr		1220
# define __NR_lgetxattr		1221
# define __NR_fgetxattr		1222
# define __NR_listxattr		1223
# define __NR_llistxattr	1224
# define __NR_flistxattr	1225
# define __NR_removexattr	1226
# define __NR_lremovexattr	1227
# define __NR_fremovexattr	1228
#elif defined (__powerpc__)
# define HAVE_XATTR_SYSCALLS 1
# define __NR_setxattr		209
# define __NR_lsetxattr		210
# define __NR_fsetxattr		211
# define __NR_getxattr		212
# define __NR_lgetxattr		213
# define __NR_fgetxattr		214
# define __NR_listxattr		215
# define __NR_llistxattr	216
# define __NR_flistxattr	217
# define __NR_removexattr	218
# define __NR_lremovexattr	219
# define __NR_fremovexattr	220
#elif defined (__x86_64__)
# define HAVE_XATTR_SYSCALLS 1
# define __NR_setxattr		188
# define __NR_lsetxattr		189
# define __NR_fsetxattr		190
# define __NR_getxattr		191
# define __NR_lgetxattr		192
# define __NR_fgetxattr		193
# define __NR_listxattr		194
# define __NR_llistxattr	195
# define __NR_flistxattr	196
# define __NR_removexattr	197
# define __NR_lremovexattr	198
# define __NR_fremovexattr	199
#elif defined (__s390__)
# define HAVE_XATTR_SYSCALLS 1
# define __NR_setxattr		224
# define __NR_lsetxattr		225
# define __NR_fsetxattr		226
# define __NR_getxattr		227
# define __NR_lgetxattr		228
# define __NR_fgetxattr		229
# define __NR_listxattr		230
# define __NR_llistxattr	231
# define __NR_flistxattr	232
# define __NR_removexattr	233
# define __NR_lremovexattr	234
# define __NR_fremovexattr	235
#elif defined (__arm__)
# define HAVE_XATTR_SYSCALLS 1
# if defined(__ARM_EABI__) || defined(__thumb__)
#  define __NR_SYSCALL_BASE 0
# else
#  define __NR_SYSCALL_BASE 0x900000
# endif
# define __NR_setxattr		(__NR_SYSCALL_BASE+226)
# define __NR_lsetxattr		(__NR_SYSCALL_BASE+227)
# define __NR_fsetxattr		(__NR_SYSCALL_BASE+228)
# define __NR_getxattr		(__NR_SYSCALL_BASE+229)
# define __NR_lgetxattr		(__NR_SYSCALL_BASE+230)
# define __NR_fgetxattr		(__NR_SYSCALL_BASE+231)
# define __NR_listxattr		(__NR_SYSCALL_BASE+232)
# define __NR_llistxattr	(__NR_SYSCALL_BASE+233)
# define __NR_flistxattr	(__NR_SYSCALL_BASE+234)
# define __NR_removexattr	(__NR_SYSCALL_BASE+235)
# define __NR_lremovexattr	(__NR_SYSCALL_BASE+236)
# define __NR_fremovexattr	(__NR_SYSCALL_BASE+237)
#elif defined (__mips64__)
# define HAVE_XATTR_SYSCALLS 1
# define __NR_Linux 5000
# define __NR_setxattr		(__NR_Linux + 217)
# define __NR_lsetxattr		(__NR_Linux + 218)
# define __NR_fsetxattr		(__NR_Linux + 219)
# define __NR_getxattr		(__NR_Linux + 220)
# define __NR_lgetxattr		(__NR_Linux + 221)
# define __NR_fgetxattr		(__NR_Linux + 222)
# define __NR_listxattr		(__NR_Linux + 223)
# define __NR_llistxattr	(__NR_Linux + 224)
# define __NR_flistxattr	(__NR_Linux + 225)
# define __NR_removexattr	(__NR_Linux + 226)
# define __NR_lremovexattr	(__NR_Linux + 227)
# define __NR_fremovexattr	(__NR_Linux + 228)
#elif defined (__mips__)
# define HAVE_XATTR_SYSCALLS 1
# define __NR_Linux 4000
# define __NR_setxattr		(__NR_Linux + 224)
# define __NR_lsetxattr		(__NR_Linux + 225)
# define __NR_fsetxattr		(__NR_Linux + 226)
# define __NR_getxattr		(__NR_Linux + 227)
# define __NR_lgetxattr		(__NR_Linux + 228)
# define __NR_fgetxattr		(__NR_Linux + 229)
# define __NR_listxattr		(__NR_Linux + 230)
# define __NR_llistxattr	(__NR_Linux + 231)
# define __NR_flistxattr	(__NR_Linux + 232)
# define __NR_removexattr	(__NR_Linux + 233)
# define __NR_lremovexattr	(__NR_Linux + 234)
# define __NR_fremovexattr	(__NR_Linux + 235)
#elif defined (__alpha__)
# define HAVE_XATTR_SYSCALLS 1
# define __NR_setxattr		382
# define __NR_lsetxattr		383
# define __NR_fsetxattr		384
# define __NR_getxattr		385
# define __NR_lgetxattr		386
# define __NR_fgetxattr		387
# define __NR_listxattr		388
# define __NR_llistxattr	389
# define __NR_flistxattr	390
# define __NR_removexattr	391
# define __NR_lremovexattr	392
# define __NR_fremovexattr	393
#elif defined (__mc68000__)
# define HAVE_XATTR_SYSCALLS 1
# define __NR_setxattr		223
# define __NR_lsetxattr		224
# define __NR_fsetxattr		225
# define __NR_getxattr		226
# define __NR_lgetxattr		227
# define __NR_fgetxattr		228
# define __NR_listxattr		229
# define __NR_llistxattr	230
# define __NR_flistxattr	231
# define __NR_removexattr	232
# define __NR_lremovexattr	233
# define __NR_fremovexattr	234
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
