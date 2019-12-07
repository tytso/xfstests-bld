/*
 * telldir.c -- Android compatibility hack because bionic doesn't
 * support telldir/seekdir
 */

#include <fcntl.h>
#include <sys/types.h>
#include <dirent.h>
#include <unistd.h>

/*
 * We're doing something evil, evil, evil, but there's no way around
 * this because bionic doesn't implment telldir/seekdir, even though
 * those interfaces are in POSIX (sob).  Fortunately (unfortunately)
 * Apportable decided to copy this structure into a commonly used NDK
 * library, so it's unlikely the first field in this structure will
 * change.  See b/210372080 or bionic/libc/bionic/dirent.cpp and the
 * comments before the definition of struct DIR in that file for more
 * details.
 *
 * The real right answer is the telldir/seekdir functions that we're
 * implementing here should be moved to bionic....
 */
struct DIR {
  int fd_;
};

long telldir(struct DIR *dirp)
{
	return (long) lseek(dirp->fd_, 0, SEEK_CUR);
}

void seekdir(DIR *dirp, long loc)
{
	(void) lseek(dirp->fd_, loc, SEEK_SET);
}
