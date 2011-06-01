/*
 * Copyright (c) 1995, 2001-2002 Silicon Graphics, Inc.
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

#include <dmapi.h>
#include <dmapi_kern.h>
#include "dmapi_lib.h"

#include <mntent.h>
#include <dirent.h>
#ifdef linux
#include "getdents.h"
#endif

static int getcomp(int dirfd, void *targhanp, size_t targhlen,
			char *bufp, size_t buflen, size_t *rlenp);
static char *get_mnt(void *fshanp, size_t fshlen);


extern int
dm_handle_to_path(
	void		*dirhanp,	/* parent directory handle and length */
	size_t		dirhlen,
	void		*targhanp,	/* target object handle and length */
	size_t		targhlen,
	size_t		buflen,		/* length of pathbufp */
	char		*pathbufp,	/* buffer in which name is returned */
	size_t		*rlenp)		/* length of resultant pathname */
{
	int		dirfd = -1;	/* fd for parent directory */
	int		origfd = -1;	/* fd for current working directory */
	int		err;		/* a place to save errno */
	int		mfd;
	char		*mtpt = NULL;
	void		*fshanp;
	size_t		fshlen;

	if (buflen == 0) {
		errno = EINVAL;
		return -1;
	}
	if (pathbufp == NULL || rlenp == NULL) {
		errno = EFAULT;
		return -1;
	}
	if (dm_handle_to_fshandle(dirhanp, dirhlen, &fshanp, &fshlen)) {
		errno = EINVAL;
		return -1;
	}
	if ((origfd = open(".", O_RDONLY)) < 0) {
		dm_handle_free(fshanp, fshlen);
		return -1;	/* leave errno set from open */
	}

	if ((mtpt = get_mnt(fshanp, fshlen)) == NULL) {
		errno = EINVAL;
		dm_handle_free(fshanp, fshlen);
		close(origfd);
		return -1;
	}

	if((mfd = open(mtpt, O_RDONLY)) < 0) {
		dm_handle_free(fshanp, fshlen);
		close(origfd);
		free(mtpt);
		return -1;
	}

	dirfd = dmi(DM_OPEN_BY_HANDLE, mfd, dirhanp, dirhlen, O_RDONLY);

	if (dirfd < 0) {
		err = errno;
	} else if (fchdir(dirfd)) {
		err = errno;
	} else {
		/* From here on the fchdir must always be undone! */

		if (!getcwd(pathbufp, buflen)) {
			if ((err = errno) == ERANGE)	/* buffer too small */
				err = E2BIG;
		} else {
			char		hbuf[DM_MAX_HANDLE_SIZE];
			size_t		hlen;

			/* Check that we're in the correct directory.
			 * If the dir we wanted has not been accessed
			 * then the kernel would have put us into the
			 * filesystem's root directory--but at least
			 * we'll be on the correct filesystem.
			 */

			err = 0;
			if (dmi(DM_PATH_TO_HANDLE, pathbufp, hbuf, &hlen)) {
				err = ENOENT;
			}
			else {
				if (dm_handle_cmp(dirhanp, dirhlen, hbuf, hlen)) {
					/* The dir we want has never been
					 * accessed, so we'll have to find
					 * it.
					 */

					/* XXX -- need something to march
					   through all the dirs, trying to
					   find the right one.  Something
					   like a recursive version of
					   getcomp().
					   In practice, are we ever going
					   to need this? */

					err = ENOENT;
				}
			}

			/* Now march through the dir to find the target. */
			if (!err) {
				err = getcomp(dirfd, targhanp, targhlen, pathbufp,
						buflen, rlenp);
			}
		}
		(void) fchdir(origfd);	/* can't do anything about a failure */
	}

	dm_handle_free(fshanp, fshlen);
	free(mtpt);
	close(mfd);
	if (origfd >= 0)
		(void)close(origfd);
	if (dirfd >= 0)
		(void)close(dirfd);
	if (!err)
		return(0);

	if (err == E2BIG)
		*rlenp = 2 * buflen;	/* guess since we don't know */
	errno = err;
	return(-1);
}


/* Append the basename of the open file referenced by targfd found in the
   directory dirfd to dirfd's pathname in bufp.  The length of the entire
   path (including the NULL) is returned in *rlenp.

   Returns zero if successful, an appropriate errno if not.
*/

#define READDIRSZ	16384

static int
getcomp(
	int		dirfd,
	void		*targhanp,
	size_t		targhlen,
	char		*bufp,
	size_t		buflen,
	size_t		*rlenp)
{
	char		buf[READDIRSZ];	/* directory entry data buffer */
	int		loc = 0;	/* byte offset of entry in the buffer */
	int		size = 0;	/* number of bytes of data in buffer */
	int		eof = 0;	/* did last ngetdents exhaust dir.? */
	struct dirent64 *dp;		/* pointer to directory entry */
	char		hbuf[DM_MAX_HANDLE_SIZE];
	size_t		hlen;
	size_t		dirlen;		/* length of dirfd's pathname */
	size_t		totlen;		/* length of targfd's pathname */
	dm_ino_t	ino;		/* target object's inode # */

	if (dm_handle_to_ino(targhanp, targhlen, &ino))
		return -1;      /* leave errno set from dm_handle_to_ino */

	/* Append a "/" to the directory name unless the directory is root. */

	dirlen = strlen(bufp);
	if (dirlen > 1) {
		if (buflen < dirlen + 1 + 1)
			return(E2BIG);
		bufp[dirlen++] = '/';
	}

	/* Examine each entry in the directory looking for one with a
	   matching target handle.
	*/

	for(;;) {
		if (size > 0) {
			dp = (struct dirent64 *)&buf[loc];
			loc += dp->d_reclen;
		}
		if (loc >= size) {
			if (eof) {
				return(ENOENT);
			}
			loc = size = 0;
		}
		if (size == 0) {	/* refill buffer */
#ifdef linux
			size = __getdents_wrap(dirfd, (char *)buf, sizeof(buf));
#else
			size = ngetdents64(dirfd, (struct dirent64 *)buf,
				    sizeof(buf), &eof);
#endif
			if (size == 0)	{	/* This also means EOF */
				return(ENOENT);
			}
			if (size < 0) {		/* error */
				return(errno);
			}
		}
		dp = (struct dirent64 *)&buf[loc];

		if (dp->d_ino != ino)
			continue;	/* wrong inode; try again */
		totlen = dirlen + strlen(dp->d_name) + 1;
		if (buflen < totlen)
			return(E2BIG);
		(void)strcpy(bufp + dirlen, dp->d_name);

		if (dmi(DM_PATH_TO_HANDLE, bufp, hbuf, &hlen))
			continue;	/* must have been removed/renamed */
		if (!dm_handle_cmp(targhanp, targhlen, hbuf, hlen))
			break;
	}

	/* We have a match based upon the target handle.  Clean up the end
	   cases before returning the path to the caller.
	*/

	if (!strcmp(dp->d_name, ".")) {		/* the directory itself */
		if (dirlen > 1)
			dirlen--;
		bufp[dirlen] = '\0';		/* trim the trailing "/." */
		*rlenp = dirlen + 1;
		return(0);
	}
	if (!strcmp(dp->d_name, "..")) {	/* the parent directory */
		char	*slash;

		if (dirlen > 1)
			dirlen--;
		bufp[dirlen] = '\0';
		if ((slash = strrchr(bufp, '/')) == NULL)
			return(ENXIO);		/* getcwd screwed up */
		if (slash == bufp)		/* don't whack "/" */
			slash++;
		*slash = '\0';			/* remove the last component */
		*rlenp = strlen(bufp) + 1;
		return(0);
	}

	*rlenp = totlen;		/* success! */
	return(0);
}


static char *
get_mnt(
	void	*fshanp,
	size_t	fshlen)
{
	FILE		*file;
	struct mntent	*mntent;
	char		*mtpt = NULL;
	void		*hanp;
	size_t		hlen;

	if ((file = setmntent("/etc/mtab", "r")) == NULL)
		return NULL;

	while((mntent = getmntent(file)) != NULL) {

		/* skip anything that isn't xfs */
		if (strcmp("xfs", mntent->mnt_type) != 0)
			continue;

		/* skip root dir */
		if (strcmp("/", mntent->mnt_dir) == 0)
			continue;

		/* skip anything that isn't dmapi */
		if ((hasmntopt(mntent, "dmapi") == 0) &&
		    (hasmntopt(mntent, "dmi") == 0) &&
		    (hasmntopt(mntent, "xdsm") == 0)) {
			continue;
		}

		/* skip anything that won't report a handle */
		if (dm_path_to_fshandle(mntent->mnt_dir, &hanp, &hlen))
			continue;

		/* is this a match? */
		if (dm_handle_cmp(fshanp, fshlen, hanp, hlen) == 0) {
			/* yes */
			mtpt = strdup(mntent->mnt_dir);
		}
		dm_handle_free(hanp, hlen);

		if (mtpt)
			break;
	}
	endmntent(file);
	return mtpt;
}
