/*
 * Copyright (c) 1995, 2001 Silicon Graphics, Inc.
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


extern dm_ssize_t
dm_read_invis(
	dm_sessid_t	sid,
	void		*hanp,
	size_t		hlen,
	dm_token_t	token,
	dm_off_t	off,
	dm_size_t	len,
	void		*bufp)
{
	return dmi(DM_READ_INVIS, sid, hanp, hlen, token, off, len, bufp);
}

extern dm_ssize_t
dm_write_invis(
	dm_sessid_t	sid,
	void		*hanp,
	size_t		hlen,
	dm_token_t	token,
	int		flags,
	dm_off_t	off,
	dm_size_t	len,
	void		*bufp)
{
	return dmi(DM_WRITE_INVIS, sid, hanp, hlen, token, flags, off, len, bufp);
}

extern int
dm_sync_by_handle(
	dm_sessid_t	sid,
	void		*hanp,
	size_t		hlen,
	dm_token_t	token)
{
	return dmi(DM_SYNC_BY_HANDLE, sid, hanp, hlen, token);
}


extern int
dm_get_dioinfo(
	dm_sessid_t	sid,
	void		*hanp,
	size_t		hlen,
	dm_token_t	token,
	dm_dioinfo_t	*diop)
{
	return dmi(DM_GET_DIOINFO, sid, hanp, hlen, token, diop);
}
