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
dm_clear_inherit(
	dm_sessid_t	sid,
	void		*hanp,
	size_t		hlen,
	dm_token_t	token,
	dm_attrname_t	*attrnamep)
{
	return dmi(DM_CLEAR_INHERIT, sid, hanp, hlen, token, attrnamep);
}


extern int
dm_get_dmattr (
	dm_sessid_t	sid,
	void		*hanp,
	size_t		hlen,
	dm_token_t	token,
	dm_attrname_t	*attrnamep,
	size_t		buflen,
	void		*bufp,
	size_t		*rlenp)
{
	return dmi(DM_GET_DMATTR, sid, hanp, hlen, token, attrnamep,
		buflen, bufp, rlenp);
}


extern int
dm_getall_dmattr (
	dm_sessid_t	sid,
	void		*hanp,
	size_t		hlen,
	dm_token_t	token,
	size_t		buflen,
	void		*bufp,
	size_t		*rlenp)
{
	return dmi(DM_GETALL_DMATTR, sid, hanp, hlen, token, buflen,
		bufp, rlenp);
}


extern int
dm_getall_inherit(
	dm_sessid_t	sid,
	void		*hanp,
	size_t		hlen,
	dm_token_t	token,
	u_int		nelem,
	dm_inherit_t	*inheritbufp,
	u_int		*nelemp)
{
	return dmi(DM_GETALL_INHERIT, sid, hanp, hlen, token, nelem,
		inheritbufp, nelemp);
}


extern int
dm_remove_dmattr (
	dm_sessid_t	sid,
	void		*hanp,
	size_t		hlen,
	dm_token_t	token,
	int		setdtime,
	dm_attrname_t	*attrnamep)
{
	return dmi(DM_REMOVE_DMATTR, sid, hanp, hlen, token, setdtime,
		attrnamep);
}


extern int
dm_set_dmattr (
	dm_sessid_t	sid,
	void		*hanp,
	size_t		hlen,
	dm_token_t	token,
	dm_attrname_t	*attrnamep,
	int		setdtime,
	size_t		buflen,
	void		*bufp)
{
	return dmi(DM_SET_DMATTR, sid, hanp, hlen, token, attrnamep,
		setdtime, buflen, bufp);
}


extern int
dm_set_inherit(
	dm_sessid_t	sid,
	void		*hanp,
	size_t		hlen,
	dm_token_t	token,
	dm_attrname_t	*attrnamep,
	mode_t		mode)
{
	return dmi(DM_SET_INHERIT, sid, hanp, hlen, token, attrnamep, mode);
}


extern int
dm_set_return_on_destroy (
	dm_sessid_t	sid,
	void		*hanp,
	size_t		hlen,
	dm_token_t	token,
	dm_attrname_t	*attrnamep,
	dm_boolean_t	enable)
{
	return dmi(DM_SET_RETURN_ON_DESTROY, sid, hanp, hlen, token,
		attrnamep, enable);
}
