NAME=libaio
SPECFILE=$(NAME).spec
VERSION=$(shell awk '/Version:/ { print $$2 }' $(SPECFILE))
RELEASE=$(shell awk '/Release:/ { print $$2 }' $(SPECFILE))
CVSTAG = $(NAME)_$(subst .,-,$(VERSION))_$(subst .,-,$(RELEASE))
RPMBUILD=$(shell `which rpmbuild >&/dev/null` && echo "rpmbuild" || echo "rpm")

prefix=/usr
includedir=$(prefix)/include
libdir=$(prefix)/lib

pkgname := libaio
ver := $(shell cat .version)

version_dirty := $(strip $(shell git diff | lsdiff | grep ".version"))
dirty := $(strip $(shell git whatchanged ${pkgname}-${ver}... 2>/dev/null))
new_changes := $(strip $(shell git diff 2>/dev/null))
tag_valid := $(strip $(shell git tag -l ${pkgname}-${ver}))

default: all

all:
	@$(MAKE) -C src

install:
	@$(MAKE) -C src install prefix=$(prefix) includedir=$(includedir) libdir=$(libdir)

check:
	@$(MAKE) -C harness check

partcheck: all
	@$(MAKE) -C harness partcheck

clean:
	@if test -f src/Makefile; then $(MAKE) -C src clean ; fi
	@$(MAKE) -C harness clean

tag: tag-checks
	git tag ${pkgname}-${ver}

archive: uptodate
	@echo "Creating archive from version ${pkgname}-${ver}:"
	git archive --format=tar --prefix=${pkgname}-${ver}/ \
		${pkgname}-${ver} | gzip > ${pkgname}-${ver}.tar.gz
tag-checks:
ifneq (${version_dirty},)
	@echo "Error: .version is not committed to the repository."
	@exit 1
endif
ifneq (${new_changes},)
	@echo "Error: there are changes in this checkout that have not been"
	@echo "committed to the repository."
	@exit 1
endif

uptodate: tag-checks
ifneq (${dirty},)
	@echo -n "Error: this branch contains changes that are not part of tag "
	@echo "${pkgname}-${ver}."
	@echo -n "Try running git-whatchanged ${pkgname}-${ver}... to see "
	@echo "the changes."
	@exit 1
endif
ifeq (${tag_valid},)
	@echo "Error: tag ${pkgname}-${ver} does not exist.  Run 'make tag'"
	@echo "and try again."
	@exit 1
endif

srpm: archive
	$(RPMBUILD) --define "_sourcedir `pwd`" --define "_srcrpmdir `pwd`" --nodeps -bs $(SPECFILE)
