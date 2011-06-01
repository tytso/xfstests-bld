/*
  File: quote.c

  Copyright (C) 2003 Andreas Gruenbacher <a.gruenbacher@computer.org>

  This program is free software; you can redistribute it and/or
  modify it under the terms of the GNU Library General Public
  License as published by the Free Software Foundation; either
  version 2 of the License, or (at your option) any later version.

  This program is distributed in the hope that it will be useful,
  but WITHOUT ANY WARRANTY; without even the implied warranty of
  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the GNU
  Library General Public License for more details.

  You should have received a copy of the GNU Library General Public
  License along with this library; if not, write to the Free Software
  Foundation, Inc., 59 Temple Place - Suite 330, Boston, MA 02111-1307, USA.
*/

#include <stdio.h>
#include <stdlib.h>
#include <ctype.h>
#include "misc.h"

const char *quote(const char *str)
{
	static char *quoted_str;
	static size_t quoted_str_len;
	const unsigned char *s;
	char *q;
	size_t nonpr;

	if (!str)
		return str;

	for (nonpr = 0, s = (unsigned char *)str; *s != '\0'; s++)
		if (!isprint(*s) || isspace(*s) || *s == '\\' || *s == '=')
			nonpr++;
	if (nonpr == 0)
		return str;

	if (high_water_alloc((void **)&quoted_str, &quoted_str_len,
			     (s - (unsigned char *)str) + nonpr * 3 + 1))
		return NULL;
	for (s = (unsigned char *)str, q = quoted_str; *s != '\0'; s++) {
		if (!isprint(*s) || isspace(*s) || *s == '\\' || *s == '=') {
			*q++ = '\\';
			*q++ = '0' + ((*s >> 6)    );
			*q++ = '0' + ((*s >> 3) & 7);
			*q++ = '0' + ((*s     ) & 7);
		} else
			*q++ = *s;
	}
	*q++ = '\0';

	return quoted_str;
}
