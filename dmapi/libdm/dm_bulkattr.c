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
dm_init_attrloc(
	dm_sessid_t	sid,
	void		*hanp,
	size_t		hlen,
	dm_token_t	token,
	dm_attrloc_t	*locp)
{
	return dmi(DM_INIT_ATTRLOC, sid, hanp, hlen, token, locp);
}

extern int
dm_get_bulkattr(
	dm_sessid_t	sid,
	void		*hanp,
	size_t		hlen,
	dm_token_t	token,
	u_int		mask,
	dm_attrloc_t	*locp,
	size_t		buflen,
	void		*bufp,
	size_t		*rlenp)
{
	return dmi(DM_GET_BULKATTR, sid, hanp, hlen, token, mask, locp, buflen, bufp, rlenp);
}

extern int
dm_get_dirattrs(
	dm_sessid_t	sid,
	void		*hanp,
	size_t		hlen,
	dm_token_t	token,
	u_int		mask,
	dm_attrloc_t	*locp,
	size_t		buflen,
	void		*bufp,
	size_t		*rlenp)
{
	return dmi(DM_GET_DIRATTRS, sid, hanp, hlen, token, mask, locp, buflen, bufp, rlenp);
}

extern int
dm_get_bulkall(
	dm_sessid_t	sid,
	void		*hanp,
	size_t		hlen,
	dm_token_t	token,
	u_int		mask,
	dm_attrname_t	*attrnamep,
	dm_attrloc_t	*locp,
	size_t		buflen,
	void		*bufp,
	size_t		*rlenp)
{
	return dmi(DM_GET_BULKALL, sid, hanp, hlen, token, mask,
			attrnamep, locp, buflen, bufp, rlenp);
}
