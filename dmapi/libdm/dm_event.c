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
dm_get_events(
	dm_sessid_t	sid,
	u_int		maxmsgs,
	u_int		flags,
	size_t		buflen,
	void		*bufp,
	size_t		*rlenp)
{
	return dmi(DM_GET_EVENTS, sid, maxmsgs, flags, buflen, bufp, rlenp);
}

extern int
dm_respond_event(
	dm_sessid_t	sid,
	dm_token_t	token,
	dm_response_t	response,
	int		reterror,
	size_t		buflen,
	void		*respbufp)
{
	return dmi(DM_RESPOND_EVENT, sid, token, response, reterror,
			buflen, respbufp);
}


extern int
dm_get_eventlist(
	dm_sessid_t	sid,
	void		*hanp,
	size_t		hlen,
	dm_token_t	token,
	u_int		nelem,
	dm_eventset_t	*eventsetp,
	u_int		*nelemp)
{
	return dmi(DM_GET_EVENTLIST, sid, hanp, hlen, token, nelem,
			eventsetp, nelemp);
}

extern int
dm_set_eventlist(
	dm_sessid_t	sid,
	void		*hanp,
	size_t		hlen,
	dm_token_t	token,
	dm_eventset_t	*eventsetp,
	u_int		maxevent)
{
	return dmi(DM_SET_EVENTLIST, sid, hanp, hlen, token,
			eventsetp, maxevent);
}

extern int
dm_set_disp(
	dm_sessid_t	sid,
	void		*hanp,
	size_t		hlen,
	dm_token_t	token,
	dm_eventset_t	*eventsetp,
	u_int		maxevent)
{
	return dmi(DM_SET_DISP, sid, hanp, hlen, token, eventsetp, maxevent);
}

extern int
dm_create_userevent(
	dm_sessid_t	sid,
	size_t		msglen,
	void		*msgdatap,
	dm_token_t	*tokenp)
{
	return dmi(DM_CREATE_USEREVENT, sid, msglen, msgdatap, tokenp);
}

extern int
dm_send_msg(
	dm_sessid_t	targetsid,
	dm_msgtype_t	msgtype,
	size_t		buflen,
	void		*bufp)
{
	return dmi(DM_SEND_MSG, targetsid, msgtype, buflen, bufp);
}

extern int
dm_move_event(
	dm_sessid_t	srcsid,
	dm_token_t	token,
	dm_sessid_t	targetsid,
	dm_token_t	*rtokenp)
{
	return dmi(DM_MOVE_EVENT, srcsid, token, targetsid, rtokenp);
}

extern int
dm_pending(
	dm_sessid_t	sid,
	dm_token_t	token,
	dm_timestruct_t	*delay)
{
	return dmi(DM_PENDING, sid, token, delay);
}

extern int
dm_getall_disp(
	dm_sessid_t	sid,
	size_t		buflen,
	void		*bufp,
	size_t		*rlenp)
{
	return dmi(DM_GETALL_DISP, sid, buflen, bufp, rlenp);
}
