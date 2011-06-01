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

#include <errno.h>
#include <stdlib.h>
#include <string.h>

#include <dmapi.h>
#include <dmapi_kern.h>
#include "dmapi_lib.h"

typedef	enum	{
	DM_HANDLE_GLOBAL,
	DM_HANDLE_FILESYSTEM,
	DM_HANDLE_FILE,
	DM_HANDLE_BAD
} dm_handletype_t;


extern int
dm_path_to_handle (
	char		*path,
	void		**hanpp,
	size_t		*hlenp)
{
	char		hbuf[DM_MAX_HANDLE_SIZE];

	if (dmi(DM_PATH_TO_HANDLE, path, hbuf, hlenp))
		return(-1);

	if ((*hanpp = malloc(*hlenp)) == NULL) {	
		errno = ENOMEM;
		return -1;
	}
	memcpy(*hanpp, hbuf, *hlenp);
	return(0);
}


extern int
dm_path_to_fshandle (
	char		*path,
	void		**hanpp,
	size_t		*hlenp)
{
	char		hbuf[DM_MAX_HANDLE_SIZE];

	if (dmi(DM_PATH_TO_FSHANDLE, path, hbuf, hlenp))
		return(-1);

	if ((*hanpp = malloc(*hlenp)) == NULL) {	
		errno = ENOMEM;
		return -1;
	}
	memcpy(*hanpp, hbuf, *hlenp);
	return(0);
}


extern int
dm_fd_to_handle (
	int		fd,
	void		**hanpp,
	size_t		*hlenp)
{
	char		hbuf[DM_MAX_HANDLE_SIZE];

	if (dmi(DM_FD_TO_HANDLE, fd, hbuf, hlenp))
		return(-1);

	if ((*hanpp = malloc(*hlenp)) == NULL) {	
		errno = ENOMEM;
		return -1;
	}
	memcpy(*hanpp, hbuf, *hlenp);
	return(0);
}


extern int
dm_handle_to_fshandle (
	void		*hanp,
	size_t		hlen,
	void		**fshanp,
	size_t		*fshlen)
{
	dm_fsid_t	fsid;

	if (dm_handle_to_fsid(hanp, hlen, &fsid))
		return(-1);

	*fshlen = sizeof(fsid);
	if ((*fshanp = malloc(*fshlen)) == NULL) {
		errno = ENOMEM;
		return(-1);
	}
	memcpy(*fshanp, &fsid, *fshlen);
	return(0);
}


/* ARGSUSED */
extern void
dm_handle_free(
	void		*hanp,
	size_t		hlen)
{
	free(hanp);
}


static dm_handletype_t
parse_handle(
	void		*hanp,
	size_t		hlen,
	dm_fsid_t	*fsidp,
	dm_ino_t	*inop,
	dm_igen_t	*igenp)
{
	dm_handle_t	handle;
	dm_fid_t	*dmfid;

	if (hanp == DM_GLOBAL_HANP && hlen == DM_GLOBAL_HLEN)
		return(DM_HANDLE_GLOBAL);

	if (hlen < sizeof(handle.ha_fsid) || hlen > sizeof(handle))
		return(DM_HANDLE_BAD);

	memcpy(&handle, hanp, hlen);
	if (! handle.ha_fsid)
		return(DM_HANDLE_BAD);
	if (fsidp)
		memcpy(fsidp, &handle.ha_fsid, sizeof(handle.ha_fsid));
	if (hlen == sizeof(handle.ha_fsid))
		return(DM_HANDLE_FILESYSTEM);

	if (handle.ha_fid.dm_fid_len != (hlen - sizeof(handle.ha_fsid) - sizeof(handle.ha_fid.dm_fid_len)))
		return(DM_HANDLE_BAD);

	dmfid = &handle.ha_fid;
	if (dmfid->dm_fid_len == sizeof *dmfid - sizeof dmfid->dm_fid_len) {
		if (dmfid->dm_fid_pad)
			return(DM_HANDLE_BAD);
		if (inop)
			*inop  = dmfid->dm_fid_ino;
		if (igenp)
			*igenp = dmfid->dm_fid_gen;
	} else {
		return(DM_HANDLE_BAD);
	}
	return(DM_HANDLE_FILE);
}


extern dm_boolean_t
dm_handle_is_valid(
	void		*hanp,
	size_t		hlen)
{
	if (parse_handle(hanp, hlen, NULL, NULL, NULL) != DM_HANDLE_BAD)
		return(DM_TRUE);
	return(DM_FALSE);
}


extern int
dm_handle_cmp(
	void		*hanp1,
	size_t		hlen1,
	void		*hanp2,
	size_t		hlen2)
{
	int		diff;

	/* If the handles don't have the same length, then this is an
	   apples-and-oranges comparison.  For lack of a better option,
	   use the handle lengths to sort them into an arbitrary order.
	*/
	if ((diff = (int)(hlen1 - hlen2)) != 0)
		return diff;
	return(memcmp(hanp1, hanp2, hlen1));
}


extern u_int
dm_handle_hash(
	void		*hanp,
	size_t		hlen)
{
	size_t		i;
	u_int		hash = 0;
	u_char		*ip = (u_char *)hanp;

	for (i = 0; i < hlen; i++) {
		hash += *ip++;
	}
	return(hash);
}


extern int
dm_handle_to_fsid(
	void		*hanp,
	size_t		hlen,
	dm_fsid_t	*fsidp)
{
	dm_handletype_t	htype;

	htype = parse_handle(hanp, hlen, fsidp, NULL, NULL);
	if (htype == DM_HANDLE_FILE || htype == DM_HANDLE_FILESYSTEM)
		return(0);
	errno = EBADF;
	return(-1);
}


extern int
dm_handle_to_ino(
	void		*hanp,
	size_t		hlen,
	dm_ino_t	*inop)
{
	if (parse_handle(hanp, hlen, NULL, inop, NULL) == DM_HANDLE_FILE)
		return(0);
	errno = EBADF;
	return(-1);
}


extern int
dm_handle_to_igen(
	void		*hanp,
	size_t		hlen,
	dm_igen_t	*igenp)
{
	if (parse_handle(hanp, hlen, NULL, NULL, igenp) == DM_HANDLE_FILE)
		return(0);
	errno = EBADF;
	return(-1);
}


extern int
dm_make_handle(
	dm_fsid_t	*fsidp,
	dm_ino_t	*inop,
	dm_igen_t	*igenp,
	void		**hanpp,
	size_t		*hlenp)
{
	dm_fid_t	*fid;
	dm_handle_t	handle;

	memcpy(&handle.ha_fsid, fsidp, sizeof(handle.ha_fsid));
	fid = &handle.ha_fid;
	fid->dm_fid_pad = 0;
	fid->dm_fid_gen = (__u32)*igenp;
	fid->dm_fid_ino = *inop;
	fid->dm_fid_len = sizeof(*fid) - sizeof(fid->dm_fid_len);
	*hlenp = sizeof(*fid) + sizeof(handle.ha_fsid);
	if ((*hanpp = malloc(*hlenp)) == NULL) {	
		errno = ENOMEM;
		return -1;
	}
	memcpy(*hanpp, &handle, *hlenp);
	return(0);
}


extern int
dm_make_fshandle(
	dm_fsid_t	*fsidp,
	void		**hanpp,
	size_t		*hlenp)
{
	*hlenp = sizeof(fsid_t);
	if ((*hanpp = malloc(*hlenp)) == NULL) {	
		errno = ENOMEM;
		return -1;
	}
	memcpy(*hanpp, fsidp, *hlenp);
	return(0);
}


extern int
dm_create_by_handle(
	dm_sessid_t	sid,
	void		*dirhanp,
	size_t		dirhlen,
	dm_token_t	token,
	void		*hanp,
	size_t		hlen,
	char		*cname)
{
	return dmi(DM_CREATE_BY_HANDLE, sid, dirhanp, dirhlen, token,
		hanp, hlen, cname);
}


extern int
dm_mkdir_by_handle(
	dm_sessid_t	sid,
	void		*dirhanp,
	size_t		dirhlen,
	dm_token_t	token,
	void		*hanp,
	size_t		hlen,
	char		*cname)
{
	return dmi(DM_MKDIR_BY_HANDLE, sid, dirhanp, dirhlen, token, hanp,
		hlen, cname);
}


extern int
dm_symlink_by_handle(
	dm_sessid_t	sid,
	void		*dirhanp,
	size_t		dirhlen,
	dm_token_t	token,
	void		*hanp,
	size_t		hlen,
	char		*cname,
	char		*path)
{
	return dmi(DM_SYMLINK_BY_HANDLE, sid, dirhanp, dirhlen, token, hanp,
		hlen, cname, path);
}
