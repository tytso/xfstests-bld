/*
 * topology.c - emulation functions for blkid v2 from util-linux
 *
 * Copyright (C) 2015 Theodore Ts'o
 *
 * %Begin-Header%
 * This file may be redistributed under the terms of the
 * GNU Lesser General Public License.
 * %End-Header%
 */

#include <unistd.h>
#include <stdlib.h>
#include <string.h>
#include <stdio.h>
#include <sys/types.h>
#include <sys/stat.h>
#include <sys/fcntl.h>
#include <sys/ioctl.h>
#include <linux/fs.h>

#include "blkidP.h"

struct blkid_struct_topology {
	unsigned int	alignment_offset;
	unsigned int	minimum_io_size;
	unsigned int	optimal_io_size;
	unsigned int	logical_sector_size;
	unsigned int	physical_sector_size;
};

struct blkid_struct_probe {
	char		*pr_name;
	blkid_cache	pr_cache;
	blkid_dev	pr_dev;
	int		pr_fd;
	struct blkid_struct_topology	pr_topology;
};

blkid_probe blkid_new_probe_from_filename(const char *filename)
{
	struct blkid_struct_probe *pr;

	pr = malloc(sizeof(struct blkid_struct_probe));
	if (!pr)
		return NULL;
	memset(pr, 0, sizeof(struct blkid_struct_probe));
	pr->pr_fd = open(filename, O_RDONLY);
	if (pr->pr_fd < 0)
		goto errout;
	pr->pr_name = blkid_strdup(filename);
	if (!pr->pr_name)
		goto errout;

	return pr;

errout:
	blkid_free_probe(pr);
	return NULL;
}

void blkid_free_probe(blkid_probe pr)
{
	if (!pr)
		return;
	if (pr->pr_fd >= 0)
		(void) close(pr->pr_fd);
	if (pr->pr_cache)
		blkid_put_cache(pr->pr_cache);
	free(pr->pr_name);
	free(pr);
}

int blkid_do_fullprobe(blkid_probe pr)
{
	int ret;
	char *type;
	
	if (!pr)
		return -1;
	if (!pr->pr_cache)
		if (blkid_get_cache(&pr->pr_cache, NULL) < 0)
			return -1;
	pr->pr_dev = blkid_get_dev(pr->pr_cache, pr->pr_name,
				   BLKID_DEV_NORMAL);
	if (!pr->pr_dev || !blkid_dev_has_tag(pr->pr_dev, "TYPE", NULL))
		return 1;	/* Nothing detected */
	return 0;
}

int blkid_probe_enable_partitions(blkid_probe pr, int enable)
{
	/* 
	 * We don't really support partition lookup, but smile and say
	 * OK
	 */
	return 0;
}

int blkid_probe_lookup_value(blkid_probe pr, const char *name,
			     const char **data, size_t *len)
{
	blkid_tag tag;
	char *ret;

	if (!pr->pr_dev)
		return -1;
	
	tag = blkid_find_tag_dev(pr->pr_dev, name);
	if (!tag)
		return -1;

	ret = blkid_strdup(tag->bit_val);
	if (!ret)
		return -1;
	if (data)
		*data = ret;
	if (len)
		*len = strlen(ret) + 1;
	return 0;
}

extern blkid_topology blkid_probe_get_topology(blkid_probe pr)
{
	int ret;
	
	if (!pr)
		return NULL;

	ret = ioctl(pr->pr_fd, BLKALIGNOFF,
		    &pr->pr_topology.alignment_offset);
	if (ret < 0)
		pr->pr_topology.alignment_offset = 0;
	
	ret = ioctl(pr->pr_fd, BLKIOMIN,
		    &pr->pr_topology.minimum_io_size);
	if (ret < 0)
		pr->pr_topology.minimum_io_size = 0;
		
	ret = ioctl(pr->pr_fd, BLKIOOPT,
		    &pr->pr_topology.optimal_io_size);
	if (ret < 0)
		pr->pr_topology.optimal_io_size = 0;

	ret = ioctl(pr->pr_fd, BLKSSZGET,
		    &pr->pr_topology.logical_sector_size);
	if (ret < 0)
		pr->pr_topology.logical_sector_size = 0;

	ret = ioctl(pr->pr_fd, BLKPBSZGET,
		    &pr->pr_topology.physical_sector_size);
	if (ret < 0)
		pr->pr_topology.physical_sector_size = 0;

	return &pr->pr_topology;
}

unsigned long blkid_topology_get_alignment_offset(blkid_topology tp)
{
	if (!tp)
		return 0;
	return tp->alignment_offset;
}

unsigned long blkid_topology_get_minimum_io_size(blkid_topology tp)
{
	if (!tp)
		return 0;
	return tp->minimum_io_size;
}
	
unsigned long blkid_topology_get_optimal_io_size(blkid_topology tp)
{
	if (!tp)
		return 0;
	return tp->optimal_io_size;
}
	
unsigned long blkid_topology_get_logical_sector_size(blkid_topology tp)
{
	if (!tp)
		return 0;
	return tp->logical_sector_size;
}

unsigned long blkid_topology_get_physical_sector_size(blkid_topology tp)
{
	if (!tp)
		return 0;
	return tp->physical_sector_size;
}

