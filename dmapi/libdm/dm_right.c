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


extern int
dm_downgrade_right(
	dm_sessid_t	sid,
	void		*hanp,
	size_t		hlen,
	dm_token_t	token)
{
	return dmi(DM_DOWNGRADE_RIGHT, sid, hanp, hlen, token);
}


extern int
dm_obj_ref_hold(
	dm_sessid_t	sid,
	dm_token_t	token,
	void		*hanp,
	size_t		hlen)
{
	return dmi(DM_OBJ_REF_HOLD, sid, token, hanp, hlen);
}


extern int
dm_obj_ref_query(
	dm_sessid_t	sid,
	dm_token_t	token,
	void		*hanp,
	size_t		hlen)
{
	return dmi(DM_OBJ_REF_QUERY, sid, token, hanp, hlen);
}


extern int
dm_obj_ref_rele(
	dm_sessid_t	sid,
	dm_token_t	token,
	void		*hanp,
	size_t		hlen)
{
	return dmi(DM_OBJ_REF_RELE, sid, token, hanp, hlen);
}


extern int
dm_query_right(
	dm_sessid_t	sid,
	void		*hanp,
	size_t		hlen,
	dm_token_t	token,
	dm_right_t	*rightp)
{
	return dmi(DM_QUERY_RIGHT, sid, hanp, hlen, token, rightp);
}


extern int
dm_release_right(
	dm_sessid_t	sid,
	void		*hanp,
	size_t		hlen,
	dm_token_t	token)
{
	return dmi(DM_RELEASE_RIGHT, sid, hanp, hlen, token);
}


extern int
dm_request_right(
	dm_sessid_t	sid,
	void		*hanp,
	size_t		hlen,
	dm_token_t	token,
	u_int		flags,
	dm_right_t	right)
{
	return dmi(DM_REQUEST_RIGHT, sid, hanp, hlen, token, flags, right);
}


extern int
dm_upgrade_right(
	dm_sessid_t	sid,
	void		*hanp,
	size_t		hlen,
	dm_token_t	token)
{
	return dmi(DM_UPGRADE_RIGHT, sid, hanp, hlen, token);
}
