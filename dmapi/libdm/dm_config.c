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
dm_get_config(
	void		*hanp,
	size_t		hlen,
	dm_config_t	flagname,
	dm_size_t	*retvalp)
{
	return dmi(DM_GET_CONFIG, hanp, hlen, flagname, retvalp);
}


extern int
dm_get_config_events(
	void		*hanp,
	size_t		hlen,
	u_int		nelem,
	dm_eventset_t	*eventsetp,
	u_int		*nelemp)
{
	return dmi(DM_GET_CONFIG_EVENTS, hanp, hlen, nelem, eventsetp, nelemp);
}
