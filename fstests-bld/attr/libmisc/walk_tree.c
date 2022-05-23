/*
  File: walk_tree.c

  Copyright (C) 2007 Andreas Gruenbacher <a.gruenbacher@computer.org>

  This program is free software; you can redistribute it and/or modify it under
  the terms of the GNU Lesser General Public License as published by the
  Free Software Foundation; either version 2.1 of the License, or (at
  your option) any later version.

  This program is distributed in the hope that it will be useful, but WITHOUT
  ANY WARRANTY; without even the implied warranty of MERCHANTABILITY or
  FITNESS FOR A PARTICULAR PURPOSE.  See the GNU Lesser General Public
  License for more details.

  You should have received a copy of the GNU Lesser General Public
  License along with this program.  If not, see <http://www.gnu.org/licenses/>.
*/

#include <sys/types.h>
#include <sys/stat.h>
#include <unistd.h>
#include <sys/time.h>
#include <sys/resource.h>
#include <dirent.h>
#include <stdio.h>
#include <string.h>
#include <errno.h>

#include "walk_tree.h"

struct entry_handle {
	struct entry_handle *prev, *next;
	dev_t dev;
	ino_t ino;
	DIR *stream;
	off_t pos;
};

struct entry_handle head = {
	.next = &head,
	.prev = &head,
	/* The other fields are unused. */
};
struct entry_handle *closed = &head;
unsigned int num_dir_handles;

static int walk_tree_visited(dev_t dev, ino_t ino)
{
	struct entry_handle *i;

	for (i = head.next; i != &head; i = i->next)
		if (i->dev == dev && i->ino == ino)
			return 1;
	return 0;
}

static int walk_tree_rec(const char *path, int walk_flags,
			 int (*func)(const char *, const struct stat *, int,
				     void *), void *arg, int depth)
{
	int follow_symlinks = (walk_flags & WALK_TREE_LOGICAL) ||
			      ((walk_flags & WALK_TREE_DEREFERENCE) &&
			       !(walk_flags & WALK_TREE_PHYSICAL) &&
			       depth == 0);
	int have_dir_stat = 0, flags = walk_flags, err;
	struct entry_handle dir;
	struct stat st;

	/*
	 * If (walk_flags & WALK_TREE_PHYSICAL), do not traverse symlinks.
	 * If (walk_flags & WALK_TREE_LOGICAL), traverse all symlinks.
	 * Otherwise, traverse only top-level symlinks.
	 */
	if (depth == 0)
		flags |= WALK_TREE_TOPLEVEL;

	if (lstat(path, &st) != 0)
		return func(path, NULL, flags | WALK_TREE_FAILED, arg);
	if (S_ISLNK(st.st_mode)) {
		flags |= WALK_TREE_SYMLINK;
		if ((flags & WALK_TREE_DEREFERENCE) ||
		    ((flags & WALK_TREE_TOPLEVEL) &&
		     (flags & WALK_TREE_DEREFERENCE_TOPLEVEL))) {
			if (stat(path, &st) != 0)
				return func(path, NULL,
					    flags | WALK_TREE_FAILED, arg);
			dir.dev = st.st_dev;
			dir.ino = st.st_ino;
			have_dir_stat = 1;
		}
	} else if (S_ISDIR(st.st_mode)) {
		dir.dev = st.st_dev;
		dir.ino = st.st_ino;
		have_dir_stat = 1;
	}
	err = func(path, &st, flags, arg);

	/*
	 * Recurse if WALK_TREE_RECURSIVE and the path is:
	 *      a dir not from a symlink
	 *      a link and follow_symlinks
	 */
        if ((flags & WALK_TREE_RECURSIVE) &&
	   (!(flags & WALK_TREE_SYMLINK) && S_ISDIR(st.st_mode)) ||
	   ((flags & WALK_TREE_SYMLINK) && follow_symlinks)) {
		struct dirent *entry;

		/*
		 * Check if we have already visited this directory to break
		 * endless loops.
		 *
		 * If we haven't stat()ed the file yet, do an opendir() for
		 * figuring out whether we have a directory, and check whether
		 * the directory has been visited afterwards. This saves a
		 * system call for each non-directory found.
		 */
		if (have_dir_stat && walk_tree_visited(dir.dev, dir.ino))
			return err;

		if (num_dir_handles == 0 && closed->prev != &head) {
close_another_dir:
			/* Close the topmost directory handle still open. */
			closed = closed->prev;
			closed->pos = telldir(closed->stream);
			closedir(closed->stream);
			closed->stream = NULL;
			num_dir_handles++;
		}

		dir.stream = opendir(path);
		if (!dir.stream) {
			if (errno == ENFILE && closed->prev != &head) {
				/* Ran out of file descriptors. */
				num_dir_handles = 0;
				goto close_another_dir;
			}

			/*
			 * PATH may be a symlink to a regular file, or a dead
			 * symlink which we didn't follow above.
			 */
			if (errno != ENOTDIR && errno != ENOENT)
				err += func(path, NULL, flags |
							WALK_TREE_FAILED, arg);
			return err;
		}

		/* See walk_tree_visited() comment above... */
		if (!have_dir_stat) {
			if (stat(path, &st) != 0)
				goto skip_dir;
			dir.dev = st.st_dev;
			dir.ino = st.st_ino;
			if (walk_tree_visited(dir.dev, dir.ino))
				goto skip_dir;
		}

		/* Insert into the list of handles. */
		dir.next = head.next;
		dir.prev = &head;
		dir.prev->next = &dir;
		dir.next->prev = &dir;
		num_dir_handles--;

		while ((entry = readdir(dir.stream)) != NULL) {
			char *path_end;

			if (!strcmp(entry->d_name, ".") ||
			    !strcmp(entry->d_name, ".."))
				continue;
			path_end = strchr(path, 0);
			if ((path_end - path) + strlen(entry->d_name) + 1 >=
			    FILENAME_MAX) {
				errno = ENAMETOOLONG;
				err += func(path, NULL,
					    flags | WALK_TREE_FAILED, arg);
				continue;
			}
			*path_end++ = '/';
			strcpy(path_end, entry->d_name);
			err += walk_tree_rec(path, walk_flags, func, arg,
					     depth + 1);
			*--path_end = 0;
			if (!dir.stream) {
				/* Reopen the directory handle. */
				dir.stream = opendir(path);
				if (!dir.stream)
					return err + func(path, NULL, flags |
						    WALK_TREE_FAILED, arg);
				seekdir(dir.stream, dir.pos);

				closed = closed->next;
				num_dir_handles--;
			}
		}

		/* Remove from the list of handles. */
		dir.prev->next = dir.next;
		dir.next->prev = dir.prev;
		num_dir_handles++;

	skip_dir:
		if (closedir(dir.stream) != 0)
			err += func(path, NULL, flags | WALK_TREE_FAILED, arg);
	}
	return err;
}

int walk_tree(const char *path, int walk_flags, unsigned int num,
	      int (*func)(const char *, const struct stat *, int, void *),
	      void *arg)
{
	char path_copy[FILENAME_MAX];

	num_dir_handles = num;
	if (num_dir_handles < 1) {
		struct rlimit rlimit;

		num_dir_handles = 1;
		if (getrlimit(RLIMIT_NOFILE, &rlimit) == 0 &&
		    rlimit.rlim_cur >= 2)
			num_dir_handles = rlimit.rlim_cur / 2;
	}
	if (strlen(path) >= FILENAME_MAX) {
		errno = ENAMETOOLONG;
		return func(path, NULL, WALK_TREE_FAILED, arg);
	}
	strcpy(path_copy, path);
	return walk_tree_rec(path_copy, walk_flags, func, arg, 0);
}
