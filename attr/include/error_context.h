#ifndef __ERROR_CONTEXT_T
#define __ERROR_CONTEXT_T

#ifdef __cplusplus
extern "C" {
#endif

struct error_context {
	/* Process an error message */
	void (*error) (struct error_context *, const char *, ...);

	/* Quote a file name for including in an error message */
	const char *(*quote) (struct error_context *, const char *);

	/* Free a quoted name */
	void (*quote_free) (struct error_context *, const char *);
};

#ifdef ERROR_CONTEXT_MACROS
# define error(ctx, args...) do { \
	if ((ctx) && (ctx)->error) \
		(ctx)->error((ctx), args); \
	} while(0)
# define quote(ctx, name) \
	( ((ctx) && (ctx)->quote) ? (ctx)->quote((ctx), (name)) : (name) )
# define quote_free(ctx, name) do { \
	if ((ctx) && (ctx)->quote_free) \
		(ctx)->quote_free((ctx), (name)); \
	} while(0)
#endif

#ifdef __cplusplus
}
#endif

#endif  /* __ERROR_CONTEXT_T */
