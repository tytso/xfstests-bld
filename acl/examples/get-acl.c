#include <stdio.h>
#include <string.h>
#include <errno.h>
#include <libgen.h>
#include <sys/stat.h>
#include <sys/acl.h>

const char *progname;

int main(int argc, char *argv[])
{
	int n, ret = 0;

	progname = basename(argv[0]);

	if (argc == 1) {
		printf("%s -- get access control lists of files\n"
			"Usage: %s file ...\n",
			progname, progname);
		return 1;
	}

	for (n = 1; n < argc; n++) {
		struct stat st;
		acl_t acl, default_acl;
		char *acl_text, *default_acl_text, *token;
		
		if (stat(argv[n], &st) != 0) {
			fprintf(stderr, "%s: %s: %s\n",
				progname, argv[n], strerror(errno));
			ret = 1;
			continue;
		}

		acl = acl_get_file(argv[n], ACL_TYPE_ACCESS);
		if (acl == NULL) {
			fprintf(stderr, "%s: getting acl of %s: %s\n",
				progname, argv[n], strerror(errno));
			ret = 1;
			continue;
		}
		acl_text = acl_to_text(acl, NULL);
		acl_free(acl);

		if (S_ISDIR(st.st_mode)) {
			default_acl = acl_get_file(argv[n], ACL_TYPE_DEFAULT);
			if (default_acl == NULL) {
				acl_free(acl_text);
				fprintf(stderr, "%s: getting default acl "
					"of %s: %s\n", progname, argv[n],
					strerror(errno));
				ret = 1;
				continue;
			}
			default_acl_text = acl_to_text(default_acl, NULL);
			acl_free(default_acl);
		}

		printf("# file: %s\n"
		       "# owner: %d\n"
		       "# group: %d\n"
		       "%s",
			argv[n], st.st_uid, st.st_gid, acl_text);

		if (S_ISDIR(st.st_mode)) {
			token = strtok(default_acl_text, "\n");
			while (token) {
				printf("default:%s\n", token);
				token = strtok(NULL, "\n");
			}
			acl_free(default_acl_text);
		}
		printf("\n");

		acl_free(acl_text);
	}
	return ret;
}

