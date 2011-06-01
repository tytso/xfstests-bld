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
dm_init_service(
	char	**versionstrpp)
{
	int ret;

	*versionstrpp = DM_VER_STR_CONTENTS;
	ret = dmi_init_service( *versionstrpp );
	return(ret);
}

extern int
dm_create_session(
	dm_sessid_t	oldsid,
	char		*sessinfop,
	dm_sessid_t	*newsidp)
{
	return dmi(DM_CREATE_SESSION, oldsid, sessinfop, newsidp);
}

extern int
dm_destroy_session(
	dm_sessid_t	sid)
{
	return dmi(DM_DESTROY_SESSION, sid);
}

extern int
dm_getall_sessions(
	u_int		nelem,
	dm_sessid_t	*sidbufp,
	u_int		*nelemp)
{
	return dmi(DM_GETALL_SESSIONS, nelem, sidbufp, nelemp);
}

extern int
dm_query_session(
	dm_sessid_t	sid,
	size_t		buflen,
	void		*bufp,
	size_t		*rlenp)
{
	return dmi(DM_QUERY_SESSION, sid, buflen, bufp, rlenp);
}

extern int
dm_getall_tokens(
	dm_sessid_t	sid,
	u_int		nelem,
	dm_token_t	*tokenbufp,
	u_int		*nelemp)
{
	return dmi(DM_GETALL_TOKENS, sid, nelem, tokenbufp, nelemp);
}

extern int
dm_find_eventmsg(
	dm_sessid_t	sid,
	dm_token_t	token,
	size_t		buflen,
	void		*bufp,
	size_t		*rlenp)
{
	return dmi(DM_FIND_EVENTMSG, sid, token, buflen, bufp, rlenp);
}
