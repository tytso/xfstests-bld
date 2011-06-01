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
dm_get_region(
	dm_sessid_t	sid,
	void		*hanp,
	size_t		hlen,
	dm_token_t	token,
	u_int		nelem,
	dm_region_t	*regbufp,
	u_int		*nelemp)
{
	return dmi(DM_GET_REGION, sid, hanp, hlen, token, nelem,
			regbufp, nelemp);
}

extern int
dm_set_region(
	dm_sessid_t	sid,
	void		*hanp,
	size_t		hlen,
	dm_token_t	token,
	u_int		nelem,
	dm_region_t	*regbufp,
	dm_boolean_t	*exactflagp)
{
	return dmi(DM_SET_REGION, sid, hanp, hlen, token, nelem,
			regbufp, exactflagp);
}
