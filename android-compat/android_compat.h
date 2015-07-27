/*
 * android_compat.h - hack so we can define things missing in the
 * bionic header files
 *
 * The bionic C library is missing a number of defines that Linux
 * programs will need.  In order to minimize the changes of xfstests
 * and xfsprogs, we put them here instead.
 */

/* Don't use this for compiles using BUILD_CC (HACK) */
#if !defined(__i386__) && !defined(__x86_64__) && !defined(ANDROID_COMPAT_H)
#define ANDROID_COMPAT_H

#include <stdlib.h>
#include <dirent.h>
#include <sys/types.h>
#include <stddef.h>

#define DEV_BSIZE 512

typedef unsigned long long ino64_t;

#ifndef S_IREAD
#define S_IREAD S_IRUSR
#endif

#ifndef S_IWRITE
#define S_IWRITE S_IWUSR
#endif

#ifdef S_IEXEC
#define S_IEXEC S_IXUSR
#endif

extern char *basename(const char *path);
struct mntent;
extern char *hasmntopt(const struct mntent *mnt, const char *opt);
extern void seekdir(DIR *dirp, long loc);
extern long telldir(DIR *dirp);
extern int sighold(int sig);
extern int sigrelse(int sig);
extern int getsubopt(char **optionp, char * const *tokens, char **valuep);
extern void *valloc(size_t size);

struct shmid_ds;
extern int shmctl(int shmid, int cmd, struct shmid_ds *buf);
extern void *shmat(int shmid, const void *shmaddr, int shmflg);
extern int shmdt(const void *shmaddr);
extern int shmget(int key, size_t size, int shmflg);

extern int sync_file_range(int fd, off64_t offset, off64_t nbytes,
                           unsigned int flags);

#endif
