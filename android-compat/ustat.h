/*
 * ustat.h
 *
 * Dummy ustat.h file for bionic
 */

#include <sys/types.h>
#include <unistd.h>

struct ustat {
	daddr_t		f_tfree;
	ino_t		f_tinode;
	char		f_fname[6];
	char		f_fpack[6];
};

int ustat(dev_t dev, struct ustat *ubuf);

/*
 * This is evil, and should be in bionic's mntent.h
 *
 * We rely here on the fact that xfsprogs's linux.c also includes stat/ustat.h
 */
#define MNTOPT_RO "ro"
