/* 8.t
- Ditto for the above three tests at the offset maximum (largest
  possible ext2/3 file size.) (8.t)
 */
#include <sys/types.h>
#include <unistd.h>

long long get_fs_limit(int fd)
{
	long long min = 0, max = 9223372036854775807LL;
	char c = 0;

	while (max - min > 1) {
		if (pwrite64(fd, &c, 1, (min + max) / 2) == -1)
			max = (min + max) / 2;
		else {
			ftruncate(fd, 0);
			min = (min + max) / 2;
		}
	}
	return max;
}

#define SET_RLIMIT(x)	do ; while (0)
#define LIMIT		get_fs_limit(rwfd)
#define FILENAME	"testdir.ext2/rwfile"

#include "common-7-8.h"
