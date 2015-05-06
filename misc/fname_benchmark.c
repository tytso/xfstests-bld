/*
 * fname_benchmark.c
 *
 * This program is a microbenchmark which measures the time it takes
 * to create, lookup, and unlink files.
 */

#include <stdio.h>
#include <unistd.h>
#include <stdlib.h>
#include <sys/types.h>
#include <sys/stat.h>
#include <sys/time.h>
#include <sys/resource.h>
#include <fcntl.h>
#include <errno.h>
#include <getopt.h>

char *buf;
int bufsize = 0;

static void file_create(int n)
{
	int	fd, ret;
	char name[256];

	sprintf(name, "f%04d", n);
	fd = open(name, O_CREAT|O_WRONLY, 0777);
	if (fd < 0) {
		fprintf(stderr, "open %s for read failed: %s\n",
			name, strerror(errno));
		exit(1);
	}
	if (bufsize) {
		ret = write(fd, buf, bufsize);
		if (ret != bufsize) {
			if (ret < 0)
				fprintf(stderr, "Writing %s failed: %s\n",
					name, strerror(errno));
			else
				fprintf(stderr, "Short write of %s: %d\n",
					name, ret);
		}
	}
	close(fd);
}

static void file_read(int n)
{
	int	fd, ret;
	char name[256];

	sprintf(name, "f%04d", n);
	fd = open(name, O_RDONLY, 0777);
	if (fd < 0) {
		fprintf(stderr, "open %s for read failed: %s\n",
			name, strerror(errno));
		exit(1);
	}
	if (bufsize) {
		ret = read(fd, buf, bufsize);
		if (ret != bufsize) {
			if (ret < 0)
				fprintf(stderr, "Reading %s failed: %s\n",
					name, strerror(errno));
			else
				fprintf(stderr, "Short read of %s: %d\n",
					name, ret);
		}
	}
	close(fd);
}

static void file_unlink(int n)
{
	int	fd, ret;
	char name[256];

	sprintf(name, "f%04d", n);
	ret = unlink(name);
	if (ret < 0) {
		fprintf(stderr, "unlink %s failed: %s\n",
			name, strerror(errno));
		exit(1);
	}
}

static void inline timeval_add(struct timeval *tv1,
			       struct timeval *tv2)
{
	tv1->tv_sec += tv2->tv_sec;
	tv1->tv_usec += tv2->tv_usec;
	if (tv1->tv_usec >= 1000000) {
		tv1->tv_usec -= 1000000;
		tv1->tv_sec++;
	}
}

static void inline timeval_sub(struct timeval *tv1,
			       struct timeval *tv2)
{
	tv1->tv_sec -= tv2->tv_sec;
	if (tv1->tv_usec < tv2->tv_usec) {
		tv1->tv_usec += 1000000;
		tv1->tv_sec--;
	}
	tv1->tv_usec -= tv2->tv_usec;
}

struct time_stat {
	struct timeval usr;
	struct timeval sys;
};

struct time_stat create_stat, lookup_stat, unlink_stat;

static void upd_stat(struct rusage *start, struct time_stat *s)
{
	struct rusage end;
	
	getrusage(RUSAGE_SELF, &end);
	timeval_sub(&end.ru_utime, &start->ru_utime);
	timeval_sub(&end.ru_stime, &start->ru_stime);
	timeval_add(&s->usr, &end.ru_utime);
	timeval_add(&s->sys, &end.ru_stime);
}

static void print_stat(const char *label, struct time_stat *s)
{
	printf("%s usr %6.4f sys %6.4f\n", label,
	       (float) s->usr.tv_sec + (float) s->usr.tv_usec / 1000000,
	       (float) s->sys.tv_sec + (float) s->sys.tv_usec / 1000000);
}

static void drop_cache(void)
{
	int fd;

	sync();
	fd = open("/proc/sys/vm/drop_caches", O_RDONLY);
	if (fd < 0) {
		perror("open of drop_caches");
		exit(1);
	}
	write(fd, "3\n", 2);
	close(fd);
}

int main(int argc, char **argv)
{
	int num_files = 1000;
	int repeat = 100;
	int c, i, j;
	int do_drop = 1;
	struct rusage r;
	struct time_stat total;

	while ((c = getopt (argc, argv, "b:n:r:d:")) != EOF)
	{
		switch(c) {
		case 'b':
			bufsize = atoi(optarg);
			if (bufsize < 0) {
				fprintf(stderr, "illegal bufsize %d\n",
					bufsize);
				exit(1);
			}
			break;
		case 'n':
			num_files = atoi(optarg);
			if (num_files <= 0) {
				fprintf(stderr, "illegal num_files %d\n",
					num_files);
				exit(1);
			}
			break;
		case 'r':
			repeat = atoi(optarg);
			if (repeat <= 0) {
				fprintf(stderr, "illegal repeat count %d\n",
					repeat);
				exit(1);
			}
			break;
		case 'd':
			do_drop = atoi(optarg);
			break;
		}
	}

	if (bufsize) {
		buf = malloc(bufsize);
		if (!buf) {
			fprintf(stderr, "failed to allocate buffer\n");
			exit(1);
		}
	}
	for (i = 0; i < repeat; i++) {
		getrusage(RUSAGE_SELF, &r);
		for (j = 0; j < num_files; j++)
			file_create(j);
		upd_stat(&r, &create_stat);

		if (do_drop)
			drop_cache();
		
		getrusage(RUSAGE_SELF, &r);
		for (j = 0; j < num_files; j++)
			file_read(j);
		upd_stat(&r, &lookup_stat);

		if (do_drop)
			drop_cache();
		getrusage(RUSAGE_SELF, &r);
		for (j = 0; j < num_files; j++)
			file_unlink(j);
		upd_stat(&r, &unlink_stat);
	}
	printf("buf_size %d num_files %d repeat %d do_drop %d\n",
	       bufsize, num_files, repeat, do_drop);
	print_stat("create", &create_stat);
	print_stat("lookup", &lookup_stat);
	print_stat("unlink", &unlink_stat);
	getrusage(RUSAGE_SELF, &r);
	total.usr = r.ru_utime;
	total.sys = r.ru_stime;
	print_stat("total", &total);
}
