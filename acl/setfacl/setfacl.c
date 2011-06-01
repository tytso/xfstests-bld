/*
  File: setfacl.c
  (Linux Access Control List Management)

  Copyright (C) 1999-2002
  Andreas Gruenbacher, <a.gruenbacher@bestbits.at>

  This program is free software; you can redistribute it and/or
  modify it under the terms of the GNU Lesser General Public
  License as published by the Free Software Foundation; either
  version 2 of the License, or (at your option) any later version.

  This program is distributed in the hope that it will be useful,
  but WITHOUT ANY WARRANTY; without even the implied warranty of
  MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the GNU
  Lesser General Public License for more details.

  You should have received a copy of the GNU Lesser General Public
  License along with this library; if not, write to the Free Software
  Foundation, Inc., 59 Temple Place - Suite 330, Boston, MA 02111-1307, USA.
*/

#include <limits.h>
#include <stdio.h>
#include <string.h>
#include <unistd.h>
#include <errno.h>
#include <sys/stat.h>
#include <dirent.h>
#include <libgen.h>
#include <getopt.h>
#include <locale.h>
#include "config.h"
#include "sequence.h"
#include "parse.h"
#include "walk_tree.h"
#include "misc.h"

extern int do_set(const char *path_p, const struct stat *stat_p, int flags, void *arg);

#define POSIXLY_CORRECT_STR "POSIXLY_CORRECT"

/* '-' stands for `process non-option arguments in loop' */
#if !POSIXLY_CORRECT
#  define CMD_LINE_OPTIONS "-:bkndm:M:x:X:RLP"
#  define CMD_LINE_SPEC "[-bkndRLP] { -m|-M|-x|-X ... } file ..."
#endif
#define POSIXLY_CMD_LINE_OPTIONS "-:bkndm:M:x:X:"
#define POSIXLY_CMD_LINE_SPEC "[-bknd] {-m|-M|-x|-X ... } file ..."

struct option long_options[] = {
#if !POSIXLY_CORRECT
	{ "set",		1, 0, 's' },
	{ "set-file",		1, 0, 'S' },

	{ "mask",		0, 0, 'r' },
	{ "recursive",		0, 0, 'R' },
	{ "logical",		0, 0, 'L' },
	{ "physical",		0, 0, 'P' },
	{ "restore",		1, 0, 'B' },
	{ "test",		0, 0, 't' },
#endif
	{ "modify",		1, 0, 'm' },
	{ "modify-file",	1, 0, 'M' },
	{ "remove",		1, 0, 'x' },
	{ "remove-file",	1, 0, 'X' },

	{ "default",		0, 0, 'd' },
	{ "no-mask",		0, 0, 'n' },
	{ "remove-all",		0, 0, 'b' },
	{ "remove-default",	0, 0, 'k' },
	{ "version",		0, 0, 'v' },
	{ "help",		0, 0, 'h' },
	{ NULL,			0, 0, 0   },
};

const char *progname;
const char *cmd_line_options, *cmd_line_spec;

int walk_flags = WALK_TREE_DEREFERENCE;
int opt_recalculate;  /* recalculate mask entry (0=default, 1=yes, -1=no) */
int opt_promote;  /* promote access ACL to default ACL */
int opt_test;  /* do not write to the file system.
                      Print what would happen instead. */
#if POSIXLY_CORRECT
const int posixly_correct = 1;  /* Posix compatible behavior! */
#else
int posixly_correct;  /* Posix compatible behavior? */
#endif
int chown_error;
int promote_warning;


static const char *xquote(const char *str)
{
	const char *q = quote(str);
	if (q == NULL) {
		fprintf(stderr, "%s: %s\n", progname, strerror(errno));
		exit(1);
	}
	return q;
}

int
has_any_of_type(
	cmd_t cmd,
	acl_type_t acl_type)
{
	while (cmd) {
		if (cmd->c_type == acl_type)
			return 1;
		cmd = cmd->c_next;
	}
	return 0;
}
	

#if !POSIXLY_CORRECT
int
restore(
	FILE *file,
	const char *filename)
{
	char *path_p;
	struct stat stat;
	uid_t uid;
	gid_t gid;
	seq_t seq = NULL;
	int line = 0, backup_line;
	int error, status = 0;

	memset(&stat, 0, sizeof(stat));

	for(;;) {
		backup_line = line;
		error = read_acl_comments(file, &line, &path_p, &uid, &gid);
		if (error < 0)
			goto fail;
		if (error == 0)
			return 0;

		if (path_p == NULL) {
			if (filename) {
				fprintf(stderr, _("%s: %s: No filename found "
						  "in line %d, aborting\n"),
					progname, xquote(filename),
					backup_line);
			} else {
				fprintf(stderr, _("%s: No filename found in "
						 "line %d of standard input, "
						 "aborting\n"),
					progname, backup_line);
			}
			goto getout;
		}

		if (!(seq = seq_init()))
			goto fail;
		if (seq_append_cmd(seq, CMD_REMOVE_ACL, ACL_TYPE_ACCESS) ||
		    seq_append_cmd(seq, CMD_REMOVE_ACL, ACL_TYPE_DEFAULT))
			goto fail;

		error = read_acl_seq(file, seq, CMD_ENTRY_REPLACE,
		                     SEQ_PARSE_WITH_PERM |
				     SEQ_PARSE_DEFAULT |
				     SEQ_PARSE_MULTI,
				     &line, NULL);
		if (error != 0) {
			fprintf(stderr, _("%s: %s: %s in line %d\n"),
			        progname, xquote(filename), strerror(errno),
				line);
			goto getout;
		}

		error = lstat(path_p, &stat);
		if (opt_test && error != 0) {
			fprintf(stderr, "%s: %s: %s\n", progname,
				xquote(path_p), strerror(errno));
			status = 1;
		}
		stat.st_uid = uid;
		stat.st_gid = gid;

		error = do_set(path_p, &stat, 0, seq);
		if (error != 0) {
			status = 1;
			goto resume;
		}

		if (!opt_test &&
		    (uid != ACL_UNDEFINED_ID || gid != ACL_UNDEFINED_ID)) {
			if (chown(path_p, uid, gid) != 0) {
				fprintf(stderr, _("%s: %s: Cannot change "
					          "owner/group: %s\n"),
					progname, xquote(path_p),
					strerror(errno));
				status = 1;
			}
		}
resume:
		if (path_p) {
			free(path_p);
			path_p = NULL;
		}
		if (seq) {
			seq_free(seq);
			seq = NULL;
		}
	}

getout:
	if (path_p) {
		free(path_p);
		path_p = NULL;
	}
	if (seq) {
		seq_free(seq);
		seq = NULL;
	}
	return status;

fail:
	fprintf(stderr, "%s: %s: %s\n", progname, xquote(filename),
		strerror(errno));
	status = 1;
	goto getout;
}
#endif


void help(void)
{
	printf(_("%s %s -- set file access control lists\n"),
		progname, VERSION);
	printf(_("Usage: %s %s\n"),
		progname, cmd_line_spec);
	printf(_(
"  -m, --modify=acl        modify the current ACL(s) of file(s)\n"
"  -M, --modify-file=file  read ACL entries to modify from file\n"
"  -x, --remove=acl        remove entries from the ACL(s) of file(s)\n"
"  -X, --remove-file=file  read ACL entries to remove from file\n"
"  -b, --remove-all        remove all extended ACL entries\n"
"  -k, --remove-default    remove the default ACL\n"));
#if !POSIXLY_CORRECT
	if (!posixly_correct) {
		printf(_(
"      --set=acl           set the ACL of file(s), replacing the current ACL\n"
"      --set-file=file     read ACL entries to set from file\n"
"      --mask              do recalculate the effective rights mask\n"));
	}
#endif
  	printf(_(
"  -n, --no-mask           don't recalculate the effective rights mask\n"
"  -d, --default           operations apply to the default ACL\n"));
#if !POSIXLY_CORRECT
	if (!posixly_correct) {
		printf(_(
"  -R, --recursive         recurse into subdirectories\n"
"  -L, --logical           logical walk, follow symbolic links\n"
"  -P, --physical          physical walk, do not follow symbolic links\n"
"      --restore=file      restore ACLs (inverse of `getfacl -R')\n"
"      --test              test mode (ACLs are not modified)\n"));
	}
#endif
	printf(_(
"      --version           print version and exit\n"
"      --help              this help text\n"));
}


int next_file(const char *arg, seq_t seq)
{
	char *line;
	int errors = 0;

	if (strcmp(arg, "-") == 0) {
		while ((line = next_line(stdin)))
			errors = walk_tree(line, walk_flags, 0, do_set, seq);
		if (!feof(stdin)) {
			fprintf(stderr, _("%s: Standard input: %s\n"),
				progname, strerror(errno));
			errors = 1;
		}
	} else {
		errors = walk_tree(arg, walk_flags, 0, do_set, seq);
	}
	return errors ? 1 : 0;
}


#define ERRNO_ERROR(s) \
	({status = (s); goto errno_error; })


int main(int argc, char *argv[])
{
	int opt;
	int saw_files = 0;
	int status = 0;
	FILE *file;
	int which;
	int lineno;
	int error;
	seq_t seq = NULL;
	int seq_cmd, parse_mode;
	
	progname = basename(argv[0]);

#if POSIXLY_CORRECT
	cmd_line_options = POSIXLY_CMD_LINE_OPTIONS;
	cmd_line_spec = _(POSIXLY_CMD_LINE_SPEC);
#else
	if (getenv(POSIXLY_CORRECT_STR))
		posixly_correct = 1;
	if (!posixly_correct) {
		cmd_line_options = CMD_LINE_OPTIONS;
		cmd_line_spec = _(CMD_LINE_SPEC);
	} else {
		cmd_line_options = POSIXLY_CMD_LINE_OPTIONS;
		cmd_line_spec = _(POSIXLY_CMD_LINE_SPEC);
	}
#endif

	setlocale(LC_CTYPE, "");
	setlocale(LC_MESSAGES, "");
	bindtextdomain(PACKAGE, LOCALEDIR);
	textdomain(PACKAGE);

	while ((opt = getopt_long(argc, argv, cmd_line_options,
		                  long_options, NULL)) != -1) {
		/* we remember the two REMOVE_ACL commands of the set
		   operations because we may later need to delete them.  */
		cmd_t seq_remove_default_acl_cmd = NULL;
		cmd_t seq_remove_acl_cmd = NULL;

		if (opt != '\1' && saw_files) {
			if (seq) {
				seq_free(seq);
				seq = NULL;
			}
			saw_files = 0;
		}
		if (seq == NULL) {
			if (!(seq = seq_init()))
				ERRNO_ERROR(1);
		}

		switch (opt) {
			case 'b':  /* remove all extended entries */
				if (seq_append_cmd(seq, CMD_REMOVE_EXTENDED_ACL,
				                        ACL_TYPE_ACCESS) ||
				    seq_append_cmd(seq, CMD_REMOVE_ACL,
				                        ACL_TYPE_DEFAULT))
					ERRNO_ERROR(1);
				break;

			case 'k':  /* remove default ACL */
				if (seq_append_cmd(seq, CMD_REMOVE_ACL,
				                        ACL_TYPE_DEFAULT))
					ERRNO_ERROR(1);
				break;

			case 'n':  /* do not recalculate mask */
				opt_recalculate = -1;
				break;

			case 'r':  /* force recalculate mask */
				opt_recalculate = 1;
				break;

			case 'd':  /*  operations apply to default ACL */
				opt_promote = 1;
				break;

			case 's':  /* set */
				if (seq_append_cmd(seq, CMD_REMOVE_ACL,
					                ACL_TYPE_ACCESS))
					ERRNO_ERROR(1);
				seq_remove_acl_cmd = seq->s_last;
				if (seq_append_cmd(seq, CMD_REMOVE_ACL,
				                        ACL_TYPE_DEFAULT))
					ERRNO_ERROR(1);
				seq_remove_default_acl_cmd = seq->s_last;

				seq_cmd = CMD_ENTRY_REPLACE;
				parse_mode = SEQ_PARSE_WITH_PERM;
				goto set_modify_delete;

			case 'm':  /* modify */
				seq_cmd = CMD_ENTRY_REPLACE;
				parse_mode = SEQ_PARSE_WITH_PERM;
				goto set_modify_delete;

			case 'x':  /* delete */
				seq_cmd = CMD_REMOVE_ENTRY;
#if POSIXLY_CORRECT
				parse_mode = SEQ_PARSE_ANY_PERM;
#else
				if (posixly_correct)
					parse_mode = SEQ_PARSE_ANY_PERM;
				else
					parse_mode = SEQ_PARSE_NO_PERM;
#endif
				goto set_modify_delete;

			set_modify_delete:
				if (!posixly_correct)
					parse_mode |= SEQ_PARSE_DEFAULT;
				if (opt_promote)
					parse_mode |= SEQ_PROMOTE_ACL;
				if (parse_acl_seq(seq, optarg, &which,
				                  seq_cmd, parse_mode) != 0) {
					if (which < 0 ||
					    (size_t) which >= strlen(optarg)) {
						fprintf(stderr, _(
							"%s: Option "
						        "-%c incomplete\n"),
							progname, opt);
					} else {
						fprintf(stderr, _(
							"%s: Option "
						        "-%c: %s near "
							"character %d\n"),
							progname, opt,
							strerror(errno),
							which+1);
					}
					status = 2;
					goto cleanup;
				}
				break;

			case 'S':  /* set from file */
				if (seq_append_cmd(seq, CMD_REMOVE_ACL,
					                ACL_TYPE_ACCESS))
					ERRNO_ERROR(1);
				seq_remove_acl_cmd = seq->s_last;
				if (seq_append_cmd(seq, CMD_REMOVE_ACL,
				                        ACL_TYPE_DEFAULT))
					ERRNO_ERROR(1);
				seq_remove_default_acl_cmd = seq->s_last;

				seq_cmd = CMD_ENTRY_REPLACE;
				parse_mode = SEQ_PARSE_WITH_PERM;
				goto set_modify_delete_from_file;

			case 'M':  /* modify from file */
				seq_cmd = CMD_ENTRY_REPLACE;
				parse_mode = SEQ_PARSE_WITH_PERM;
				goto set_modify_delete_from_file;

			case 'X':  /* delete from file */
				seq_cmd = CMD_REMOVE_ENTRY;
#if POSIXLY_CORRECT
				parse_mode = SEQ_PARSE_ANY_PERM;
#else
				if (posixly_correct)
					parse_mode = SEQ_PARSE_ANY_PERM;
				else
					parse_mode = SEQ_PARSE_NO_PERM;
#endif
				goto set_modify_delete_from_file;

			set_modify_delete_from_file:
				if (!posixly_correct)
					parse_mode |= SEQ_PARSE_DEFAULT;
				if (opt_promote)
					parse_mode |= SEQ_PROMOTE_ACL;
				if (strcmp(optarg, "-") == 0) {
					file = stdin;
				} else {
					file = fopen(optarg, "r");
					if (file == NULL) {
						fprintf(stderr, "%s: %s: %s\n",
							progname,
							xquote(optarg),
							strerror(errno));
						status = 2;
						goto cleanup;
					}
				}

				lineno = 0;
				error = read_acl_seq(file, seq, seq_cmd,
				                     parse_mode, &lineno, NULL);
				
				if (file != stdin) {
					fclose(file);
				}

				if (error) {
					if (!errno)
						errno = EINVAL;

					if (file != stdin) {
						fprintf(stderr, _(
							"%s: %s in line "
						        "%d of file %s\n"),
							progname,
							strerror(errno),
							lineno,
							xquote(optarg));
					} else {
						fprintf(stderr, _(
							"%s: %s in line "
						        "%d of standard "
							"input\n"), progname,
							strerror(errno),
							lineno);
					}
					status = 2;
					goto cleanup;
				}
				break;


			case '\1':  /* file argument */
				if (seq_empty(seq))
					goto synopsis;
				saw_files = 1;

				status = next_file(optarg, seq);
				break;

			case 'B':  /* restore ACL backup */
				saw_files = 1;

				if (strcmp(optarg, "-") == 0)
					file = stdin;
				else {
					file = fopen(optarg, "r");
					if (file == NULL) {
						fprintf(stderr, "%s: %s: %s\n",
							progname,
							xquote(optarg),
							strerror(errno));
						status = 2;
						goto cleanup;
					}
				}

				status = restore(file,
				               (file == stdin) ? NULL : optarg);

				if (file != stdin)
					fclose(file);
				if (status != 0)
					goto cleanup;
				break;

			case 'R':  /* recursive */
				walk_flags |= WALK_TREE_RECURSIVE;
				break;

			case 'L':  /* follow symlinks */
				walk_flags |= WALK_TREE_LOGICAL;
				walk_flags &= ~WALK_TREE_PHYSICAL;
				break;

			case 'P':  /* do not follow symlinks */
				walk_flags |= WALK_TREE_PHYSICAL;
				walk_flags &= ~WALK_TREE_LOGICAL;
				break;

			case 't':  /* test mode */
				opt_test = 1;
				break;

			case 'v':  /* print version and exit */
				printf("%s " VERSION "\n", progname);
				status = 0;
				goto cleanup;

			case 'h':  /* help! */
				help();
				status = 0;
				goto cleanup;

			case ':':  /* option missing */
			case '?':  /* unknown option */
			default:
				goto synopsis;
		}
		if (seq_remove_acl_cmd) {
			/* This was a set operation. Check if there are
			   actually entries of ACL_TYPE_ACCESS; if there
			   are none, we need to remove this command! */
			if (!has_any_of_type(seq_remove_acl_cmd->c_next,
				            ACL_TYPE_ACCESS))
				seq_delete_cmd(seq, seq_remove_acl_cmd);
		}
		if (seq_remove_default_acl_cmd) {
			/* This was a set operation. Check if there are
			   actually entries of ACL_TYPE_DEFAULT; if there
			   are none, we need to remove this command! */
			if (!has_any_of_type(seq_remove_default_acl_cmd->c_next,
				            ACL_TYPE_DEFAULT))
				seq_delete_cmd(seq, seq_remove_default_acl_cmd);
		}
	}
	while (optind < argc) {
		if (seq_empty(seq))
			goto synopsis;
		saw_files = 1;

		status = next_file(argv[optind++], seq);
	}
	if (!saw_files)
		goto synopsis;

	goto cleanup;

synopsis:
	fprintf(stderr, _("Usage: %s %s\n"),
		progname, cmd_line_spec);
	fprintf(stderr, _("Try `%s --help' for more information.\n"),
		progname);
	status = 2;
	goto cleanup;

errno_error:
	fprintf(stderr, "%s: %s\n", progname, strerror(errno));
	goto cleanup;

cleanup:
	if (seq)
		seq_free(seq);
	return status;
}

