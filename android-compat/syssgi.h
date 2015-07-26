/*
 * syssgi.h - hack so we can define things missing in the bionic header files
 *
 * The bionic C library is missing a number of defines that Linux
 * programs will need.  In order to minimize the changes of xfstests
 * and xfsprogs, we put them here instead (since syssgi.h doesn't
 * exist on Linux or Android systems).
 */

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
