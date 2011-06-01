#ifndef __LIBATTR_H
#define __LIBATTR_H

#ifdef __cplusplus
extern "C" {
#endif

struct error_context;

extern int attr_copy_file (const char *, const char *,
			   int (*) (const char *, struct error_context *),
			   struct error_context *);
extern int attr_copy_fd (const char *, int, const char *, int,
			 int (*) (const char *, struct error_context *),
			 struct error_context *);

/* Keep this function for backwards compatibility. */
extern int attr_copy_check_permissions(const char *, struct error_context *);

#define ATTR_ACTION_SKIP	1
#define ATTR_ACTION_PERMISSIONS	2

extern int attr_copy_action(const char *, struct error_context *);

#ifdef __cplusplus
}
#endif

#endif
