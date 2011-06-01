/* 15.t
- pwritev and preadv tests.
*/
#include "aio_setup.h"
#include <sys/mman.h>
#include <sys/uio.h>
#include <errno.h>

int test_main(void)
{
#define SIZE	512
#define NUM_IOV	10
	char buf[SIZE*NUM_IOV];
	struct iovec iov[NUM_IOV];
	int rwfd;
	int	status = 0, res, i;

	rwfd = open("testdir/rwfile", O_RDWR);		assert(rwfd != -1);
	res = ftruncate(rwfd, sizeof(buf));		assert(res == 0);

	for (i = 0; i < NUM_IOV; i++) {
		iov[i].iov_base = buf + i*SIZE;
		iov[i].iov_len = SIZE;
		memset(iov[i].iov_base, i, SIZE);
	}
	status |= attempt_rw(rwfd, iov, NUM_IOV,  0, WRITEV, SIZE*NUM_IOV);
	res = pread(rwfd, buf, sizeof(buf), 0);	assert(res == sizeof(buf));
	for (i = 0; i < NUM_IOV; i++) {
		unsigned int j;
		for (j = 0; j < SIZE; j++) {
			if (buf[i*SIZE + j] != i) {
				printf("Unexpected value after writev at %i\n",
				       i*SIZE + j);
				status |= 1;
				break;
			}
		}
	}
	if (!status)
		printf("Checking memory: [Success]\n");

	memset(buf, 0, sizeof(buf));
	status |= attempt_rw(rwfd, iov, NUM_IOV,  0,  READV, SIZE*NUM_IOV);
	for (i = 0; i < NUM_IOV; i++) {
		unsigned int j;
		for (j = 0; j < SIZE; j++) {
			if (buf[i*SIZE + j] != i) {
				printf("Unexpected value after readv at %i\n",
				       i*SIZE + j);
				status |= 1;
				break;
			}
		}
	}

	/* Check that offset works. */
	status |= attempt_rw(rwfd, iov+1, NUM_IOV-1,  SIZE, WRITEV,
			     SIZE*(NUM_IOV-1));
	memset(buf, 0, sizeof(buf));
	res = pread(rwfd, buf, sizeof(buf), 0);	assert(res == sizeof(buf));
	for (i = 1; i < NUM_IOV; i++) {
		unsigned int j;
		for (j = 0; j < SIZE; j++) {
			if (buf[i*SIZE + j] != i) {
				printf("Unexpected value after offset writev at %i\n",
				       i*SIZE + j);
				status |= 1;
				break;
			}
		}
	}
	if (!status)
		printf("Checking memory: [Success]\n");

	memset(buf, 0, sizeof(buf));
	status |= attempt_rw(rwfd, iov+1, NUM_IOV-1,  SIZE, READV,
			     SIZE*(NUM_IOV-1));
	for (i = 1; i < NUM_IOV; i++) {
		unsigned int j;
		for (j = 0; j < SIZE; j++) {
			if (buf[i*SIZE + j] != i) {
				printf("Unexpected value after offset readv at %i\n",
				       i*SIZE + j);
				status |= 1;
				break;
			}
		}
	}
	if (!status)
		printf("Checking memory: [Success]\n");

	return status;
}

