/*
 * syncfs.c -- issue 
 */

#define _GNU_SOURCE

#include <stdio.h>
#include <unistd.h>
#include <stdlib.h>
#include <errno.h>
#include <sys/types.h>
#include <sys/stat.h>
#include <fcntl.h>

const char *progname;

static void usage(void)
{
	fprintf(stderr, "Usage: %s <file>\n");
	exit(1);
}

int main(int argc, char **argv)
{
	int fd;
	
	progname = argv[0];
	if (argc != 2)
		usage();
	fd = open(argv[1], O_RDONLY);
	if (fd < 0) {
		perror(argv[1]);
		exit(1);
	}
	if (syncfs(fd) < 0) {
		perror("syncfs");
		exit(1);
	}
	return 0;
}
	
