#include <stdio.h>
#include <string.h>
#include <limits.h>
#include <unistd.h>
#include "misc.h"

#define LINE_SIZE getpagesize()

char *next_line(FILE *file)
{
	static char *line;
	static size_t line_size;
	char *c;
	int eol = 0;

	if (!line) {
		if (high_water_alloc((void **)&line, &line_size, LINE_SIZE))
			return NULL;
	}
	c = line;
	do {
		if (!fgets(c, line_size - (c - line), file))
			return NULL;
		c = strrchr(c, '\0');
		while (c > line && (*(c-1) == '\n' || *(c-1) == '\r')) {
			c--;
			*c = '\0';
			eol = 1;
		}
		if (feof(file))
			break;
		if (!eol) {
			if (high_water_alloc((void **)&line, &line_size,
					     2 * line_size))
				return NULL;
			c = strrchr(line, '\0');
		}
	} while (!eol);
	return line;
}
