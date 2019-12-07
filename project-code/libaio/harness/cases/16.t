/* 16.t
- eventfd tests.
*/
#include <stdint.h>
#include <err.h>
#include <sys/syscall.h>   /* For SYS_xxx definitions */

#ifndef SYS_eventfd
#if defined(__i386__)
#define SYS_eventfd 323
#elif defined(__x86_64__)
#define SYS_eventfd 284
#elif defined(__ia64__)
#define SYS_eventfd 1309
#elif defined(__PPC__)
#define SYS_eventfd 307
#elif defined(__s390__)
#define SYS_eventfd 318
#elif defined(__alpha__)
#define SYS_eventfd 478
#else
#error define SYS_eventfd for your arch!
#endif
#endif

int test_main(void)
{
	/* 10 MB takes long enough that we would fail if eventfd
	 * returned immediately. */
#define SIZE	10000000
	char *buf;
	struct io_event io_event;
	struct iocb iocb;
	struct iocb *iocbs[] = { &iocb };
	int rwfd, efd;
	int res;
	io_context_t	io_ctx;
	uint64_t event;
	struct timespec	notime = { .tv_sec = 0, .tv_nsec = 0 };

	buf = malloc(SIZE);				assert(buf);
	efd = syscall(SYS_eventfd, 0);
	if (efd < 0) {
		if (errno == ENOSYS) {
			printf("No eventfd support.  [SKIPPING]\n");
			exit(0);
		}
		err(1, "Failed to get eventfd");
	}

	rwfd = open("testdir/rwfile", O_RDWR);		assert(rwfd != -1);
	res = ftruncate(rwfd, 0);			assert(res == 0);
	memset(buf, 0x42, SIZE);

	/* Write test. */
	res = io_queue_init(1024, &io_ctx);		assert(res == 0);
	io_prep_pwrite(&iocb, rwfd, buf, SIZE, 0);
	io_set_eventfd(&iocb, efd);
	res = io_submit(io_ctx, 1, iocbs);		assert(res == 1);

	alarm(30);
	res = read(efd, &event, sizeof(event));		assert(res == sizeof(event));
	assert(event == 1);

	/* This should now be ready. */
	res = io_getevents(io_ctx, 0, 1, &io_event, &notime);
	if (res != 1)
		err(1, "io_getevents did not return 1 event after eventfd");
	assert(io_event.res == SIZE);
	printf("eventfd write test [SUCCESS]\n");

	/* Read test. */
	memset(buf, 0, SIZE);
	io_prep_pread(&iocb, rwfd, buf, SIZE, 0);
	io_set_eventfd(&iocb, efd);
	res = io_submit(io_ctx, 1, iocbs);		assert(res == 1);

	alarm(30);
	res = read(efd, &event, sizeof(event));		assert(res == sizeof(event));
	assert(event == 1);

	/* This should now be ready. */
	res = io_getevents(io_ctx, 0, 1, &io_event, &notime);
	if (res != 1)
		err(1, "io_getevents did not return 1 event after eventfd");
	assert(io_event.res == SIZE);

	for (res = 0; res < SIZE; res++)
		assert(buf[res] == 0x42);
	printf("eventfd read test  [SUCCESS]\n");

	return 0;
}

