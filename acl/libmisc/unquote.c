/*
  File: unquote.c

  Copyright (C) 2003 Andreas Gruenbacher <a.gruenbacher@bestbits.at>

  This program is free software; you can redistribute it and/or
  modify it under the terms of the GNU Lesser General Public
  License as published by the Free Software Foundation; either
  version 2 of the License, or (at your option) any later version.

  This program is distributed in the hope that it will be useful,
  but WITHOUT ANY WARRANTY; without even the implied warranty of
  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the GNU
  Lesser General Public License for more details.

  You should have received a copy of the GNU Lesser General Public
  License along with this library; if not, write to the Free Software
  Foundation, Inc., 59 Temple Place - Suite 330, Boston, MA 02111-1307, USA.
*/

#include <stdio.h>
#include <stdlib.h>
#include <ctype.h>
#include "misc.h"

char *unquote(char *str)
{
	unsigned char *s, *t;

	if (!str)
		return str;

	for (s = (unsigned char *)str; *s != '\0'; s++)
		if (*s == '\\')
			break;
	if (*s == '\0')
		return str;

#define isoctal(c) \
	((c) >= '0' && (c) <= '7')

	t = s;
	do {
		if (*s == '\\' &&
		    isoctal(*(s+1)) && isoctal(*(s+2)) && isoctal(*(s+3))) {
			*t++ = ((*(s+1) - '0') << 6) +
			       ((*(s+2) - '0') << 3) +
			       ((*(s+3) - '0')     );
			s += 3;
		} else
			*t++ = *s;
	} while (*s++ != '\0');

	return str;
}
