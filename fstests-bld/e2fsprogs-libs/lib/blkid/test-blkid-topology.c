/*
 * test-blkid-topology.c - sample use cases of the new blkid v2 functions
 *
 * (at least those functions used by xfsprogs and e2fsprogs)
 *
 * Copyright (C) 2015 Theodore Ts'o
 *
 * %Begin-Header%
 * This file may be redistributed under the terms of the
 * GNU Lesser General Public License.
 * %End-Header%
 */


#include <stdio.h>
#include <unistd.h>
#include <stdlib.h>
#include <string.h>
#include <strings.h>
#include <ctype.h>
#include <blkid/blkid.h>
	
int main(int argc, char **argv)
{
	unsigned int blocksize;
	blkid_probe pr;
	blkid_topology tp;
	unsigned long min_io, opt_io, lsz, psz;
	char *device;
	const char *type;
	int ret;

	device = argv[1];
	pr = blkid_new_probe_from_filename(device);
	if (!pr) {
		fprintf(stderr, "blkid_new_probe_from_filename failed\n");
		exit(1);
	}
	tp = blkid_probe_get_topology(pr);
	if (!tp) {
		fprintf(stderr, "blkid_probe_get_topology\n");
		exit(1);
	}

	min_io = blkid_topology_get_minimum_io_size(tp);
	opt_io = blkid_topology_get_optimal_io_size(tp);
	lsz = blkid_topology_get_logical_sector_size(tp);
	psz = blkid_topology_get_physical_sector_size(tp);

	printf("lsz %lu psz %lu min_io %lu opt_io %lu\n", lsz, psz,
	       min_io, opt_io);

	ret = blkid_probe_enable_partitions(pr, 1);
	if (ret < 0) {
		fprintf(stderr, "blkid_probe_enable_partitions failed\n");
		exit(1);
	}

	ret = blkid_do_fullprobe(pr);
	if (ret < 0) {
		fprintf(stderr, "blkid_do_fullprobe failed\n");
		exit(1);
	}

	if (ret == 0) {
		if (!blkid_probe_lookup_value(pr, "TYPE", &type, NULL)) {
			printf("%s appears to contain an existing "
			       "filesystem (%s).\n", device, type);
		} else if (!blkid_probe_lookup_value(pr, "PTTYPE",
						     &type, NULL)) {
			printf("%s appears to contain a partition "
			       "table (%s).\n", device, type);
 		} else {
			printf("%s contains something, but it's not\n"
			       "\ta partition or file system?\n", device);
		}
		blkid_free_probe(pr);
	}

	exit(0);
}
