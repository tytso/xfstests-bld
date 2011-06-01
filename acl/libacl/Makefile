#
# Copyright (c) 2000-2002 Silicon Graphics, Inc.  All Rights Reserved.
#

TOPDIR = ..

LTLDFLAGS += -Wl,--version-script,$(TOPDIR)/exports
include $(TOPDIR)/include/builddefs

LTLIBRARY = libacl.la
LTLIBS = -lattr $(LIBMISC)
LTDEPENDENCIES = $(LIBMISC)
LT_CURRENT = 2
LT_REVISION = 0
LT_AGE = 1

CFILES = $(POSIX_CFILES) $(LIBACL_CFILES) $(INTERNAL_CFILES) \
	 perm_copy_fd.c perm_copy_file.c
HFILES = libobj.h libacl.h byteorder.h __acl_from_xattr.h __acl_to_xattr.h \
	 perm_copy.h

LCFLAGS = -include perm_copy.h

POSIX_CFILES = \
	acl_add_perm.c acl_calc_mask.c acl_clear_perms.c acl_copy_entry.c \
	acl_copy_ext.c acl_copy_int.c acl_create_entry.c acl_delete_def_file.c \
	acl_delete_entry.c acl_delete_perm.c acl_dup.c acl_free.c \
	acl_from_text.c acl_get_entry.c acl_get_fd.c acl_get_file.c \
	acl_get_perm.c acl_get_permset.c acl_get_qualifier.c \
	acl_get_tag_type.c acl_init.c acl_set_fd.c acl_set_file.c \
	acl_set_permset.c acl_set_qualifier.c acl_set_tag_type.c acl_to_text.c \
	acl_valid.c acl_size.c

LIBACL_CFILES = \
	acl_to_any_text.c acl_entries.c acl_check.c acl_error.c acl_cmp.c \
	acl_extended_fd.c acl_extended_file.c acl_equiv_mode.c acl_from_mode.c

INTERNAL_CFILES = \
	__acl_to_any_text.c __acl_to_xattr.c __acl_from_xattr.c \
	__acl_reorder_obj_p.c __libobj.c __apply_mask_to_mode.c


default: $(LTLIBRARY)

include $(BUILDRULES)

install:

install-dev: default
	$(INSTALL_LTLIB_DEV)

install-lib: default
	$(INSTALL_LTLIB)
