#include <stdio.h>
#include <string.h>
#include <errno.h>
#include <libgen.h>
#include <sys/acl.h>

const char *progname;

int main(int argc, char *argv[])
{
	acl_t acl;
	int n, ret = 0;

	progname = basename(argv[0]);

	if (argc < 3) {
		printf("%s -- set access control list of files\n"
			"Usage: %s acl file ...\n",
			progname, progname);
		return 1;
	}

	acl = acl_from_text(argv[1]);
	if (!acl) {
		fprintf(stderr, "%s: `%s': %s\n",
			progname, argv[1], strerror(errno));
		return 1;
	}
	if (acl_valid(acl) != 0) {
		fprintf(stderr, "%s: `%s': invalid/incomplete acl\n",
			progname, argv[1]);
		acl_free(acl);
		return 1;
	}

	for (n = 2; n < argc; n++) {
		if (acl_set_file(argv[n], ACL_TYPE_ACCESS, acl) != 0) {
			fprintf(stderr, "%s: setting acl of %s: %s\n",
				progname, argv[n], strerror(errno));
			ret = 1;
		}
	}

	acl_free(acl);

	return ret;
}
