"""gce-xfstests command parsing module.

The LTMParser parses gce-xfstests command strings. It mostly is concerned with
expanding fstest configuration options (like smoke, quick, auto), and with
parsing the configurations of the tests.

The parser on construction takes in a gce-xfstests commandline and
parses it. The main object attributes of concern to the Sharder are
"fsconfigs" and "extra_cmds".

- "fsconfigs" will be a dictionary, where
keys correspond to filesystem names, and the value is a list of configurations
to be run on that filesystem. Duplicates of a configuration will not be present
for a given filesystem.

- "extra_cmds" will be a list of command line arguments that were passed in that
weren't related to configurations (test sets, test set excludes, etc).
Arguments that don't make sense on the LTM (e.g. ltm, --instance-name) will
be removed before construction is complete.

The configurations specified are part of xfstests_bld, and are packaged as
part of the test appliance under /root/fs/*
Something left open to future implementation is the parsing of test sets,
as -g auto runs can still take quite a while (1-2 hours) on a single VM.


NOTES:
The most probable use case is to split a "-c all"
gce-xfstests command into one for each config, or to split a
"-g quick" or "-g auto" into a smaller number of individual xfstests, such as
"-c 4k generic/001 generic/002 generic/003 [etc...]"

Things to note:
  The argument after -c are filesystem configs.
    "fstestcfg"
    Examples of this are ext4/4k

    ** If no -c option is specified (-c <something> or smoke), it defaults to
      "all". This ends up running the test as "PRIMARY_FSTYPE"/cfg/all.list.

    PRIMARY_FSTYPE is usually ext4

    Confusing part for -c options is that they can be either a filesystem,
    a configuration, or both (in the form filesystem/configuration).
    They can also have commas between them: -c CFG1,CFG2,...

      If a / is present, parse_cli checks /root/fs/ for an existing
      folder for the part before the /, sources /root/fs/<firstpart>/config,
      and then runs <secondpart> through test_name_alias.
      It then checks for an existing file /root/fs/<firstpart>/cfg/<secondpart>
      and confirms this way that <firstpart> is a filesystem, and <secondpart>
      is a configuration for that given filesystem.

      If no / is present, parse_cli checks if /root/fs/ contains a directory
      for the string. otherwise, it sets the filesystem to PRIMARY_FSTYPE
      and the configuration to "arg" before validating the config.

      parse_cli just ensures that runtests.sh can work with the given
      arguments.

    What this function can work with is if it finds a slash, we can
    take the second part and check if it has a file <2nd>.list under
    /root/fs/<firstpart>/cfg/. If so, parse out the .list file and
    create a different config (<firstpart>/<one entry>) for each
    child VM. Otherwise, it's a single config, and we can just run
    one test for it.

    If not, it has to check if it's a filesystem. If so, we need to
    find test_name_alias for "default"
      If it isn't a filesystem, we have to check the PRIMARY_FSTYPE's
      directory for a valid fs configuration

  -g can specify a filesystem test group.
    "fstestset"
    Groups are under /root/xfstests/tests/<filesystem>/group, and
    /root/xfstests/tests/generic/group
    Each line contains the test number, followed by the groups
    that the test belongs to. Some examples are "quick", "auto", "fuzzers",
    "defrag", "resize", "rw", "punch"

    "fstestopt" can affect what tests are being run.

  -x can exclude a test group

  -X can exclude a specific test.

  Special command line args include:
    full (-g auto), quick (-g quick), smoke (-c ext4/4k -g quick),

  Other command line args we could exclude: launch, shell, maint, ver

"""
import logging
import os


class LTMParser(object):
  """Main gce-xfstests parsing class for the LTM."""

  def __init__(self, orig_cmd, default_fstype='ext4', xfs_path='/root/'):
    if not isinstance(orig_cmd, basestring):
      raise TypeError(orig_cmd)
    if not os.path.isdir(xfs_path + 'fs'):
      raise ValueError
    if not os.path.isdir(xfs_path + 'fs/' + default_fstype):
      raise ValueError
    logging.debug('LTMParser init entered.')
    logging.info('orig_cmd: %s', orig_cmd)
    self.default_fstype = default_fstype
    self.xfs_path = xfs_path
    self.orig_cmd = orig_cmd

    # After init, self.extra_cmds will be all unprocessed cmdline options.
    # (without 'smoke', 'quick', 'full', '-c', 'ltm')
    self.extra_cmds = orig_cmd.strip().split(' ')
    self.orig_cmds = list(self.extra_cmds)
    self.fsconfigs = {}
    self.removedopts = []
    self.expandedopts = []
    self.process_cmds()

  def process_cmds(self):
    if '--no-action' in self.extra_cmds:
      return
    self.sanitize_cmd_list()
    self.expand_aliases()
    self.process_configs()

  def sanitize_cmd_list(self):
    """Removes unnecessary commands from the extra commands list.

    Some commands/options for gce-xfstests don't make sense to be run from the
    LTM, or will clash with options that the LTM wants to explicitly specify.
    This procedure sanitizes the extra commands list for those.
    """
    # Remove options without arguments (just append all matching elements
    # to removedopts, and then remove from the extra_cmds list)
    no_arg_opts = {'ltm', '--no-region-shard', '--no-email'}
    self.removedopts.extend([x for x in self.extra_cmds if x in no_arg_opts])
    self.extra_cmds[:] = [x for x in self.extra_cmds if x not in no_arg_opts]

    def remove_opt_with_arg(opt_name):
      try:
        inst_ind = self.extra_cmds.index(opt_name)
        self.removedopts.append(self.extra_cmds[inst_ind])
        del self.extra_cmds[inst_ind]
        self.removedopts.append(self.extra_cmds[inst_ind])
        del self.extra_cmds[inst_ind]
      except (ValueError, IndexError):
        pass
    remove_opt_with_arg('--instance-name')
    remove_opt_with_arg('--bucket-subdir')
    remove_opt_with_arg('--gs-bucket')
    remove_opt_with_arg('--email')
    remove_opt_with_arg('--gce-zone')
    remove_opt_with_arg('--image-project')
    remove_opt_with_arg('--testrunid')
    remove_opt_with_arg('--hooks')
    remove_opt_with_arg('--update-xfstests-tar')
    remove_opt_with_arg('--update-xfstests')
    remove_opt_with_arg('--update-files')
    remove_opt_with_arg('-n')  # number of cpus
    remove_opt_with_arg('-r')  # ram
    remove_opt_with_arg('--machtype')
    remove_opt_with_arg('--kernel')

  def process_configs(self):
    """Parses the config options specified on the command line.

    This function is the main bulk of work to be done. The gce-xfstests config
    options specified as an argument to the "-c" option will be parsed and
    verified to be valid commands, and "all" options will be expanded into
    individual configs.

    After this method, the fsconfigs attribute should contain the fully
    expanded set of configs to be run from the given original command.
    """
    try:
      cfg_cmd_ind = self.extra_cmds.index('-c')
      conf = self.extra_cmds[cfg_cmd_ind + 1]
      del self.extra_cmds[cfg_cmd_ind]
      del self.extra_cmds[cfg_cmd_ind]
    except (ValueError, IndexError):
      # Without a -c configuration option, use default fs and "all.list"
      with open(self.xfs_path +
                'fs/%s/cfg/all.list' % self.default_fstype, 'r') as f:
        self.fsconfigs[self.default_fstype] = [x.strip()
                                               for x in f.readlines()]
        return

    for c in conf.strip().split(','):
      self.process_config(c)
    return

  def process_config(self, c):
    """Processes a single config option.

    Examples of arguments used include "ext4/4k", "4k", "ext4", "overlay".
    If the config option is not explicit about fs/cfg, the xfstests_bld files
    present in the test appliance will be searched and parsed to figure out
    whether the argument is a filesystem, a config, or neither.
    Otherwise, the function will verify that a config option exists before
    adding it to the fsconfigs attribute.

    Args:
      c: a single config option. These are normally separated on the command
         line by commas, e.g. "ext4/4k,overlay/small" has 2 of these.
    """
    if '/' in c:
      (fs, cfg) = c.split('/')
      if os.path.isfile('%s/fs/%s/cfg/%s.list' % (self.xfs_path, fs, cfg)):
        with open('%s/fs/%s/cfg/%s.list' % (self.xfs_path, fs, cfg)) as f:
          cfgl = [x.strip() for x in f.readlines()]
      elif os.path.isfile('%s/fs/%s/cfg/%s' % (self.xfs_path, fs, cfg)):
        cfgl = [cfg]
      else:
        return
    else:
      if os.path.isdir('%s/fs/%s' % (self.xfs_path, c)):
        fs = c
        cfgl = ['default']
      else:
        # default to ext4.
        fs = self.default_fstype
        if os.path.isfile('%s/fs/%s/cfg/%s.list' % (self.xfs_path, fs, c)):
          with open('%s/fs/%s/cfg/%s.list' % (self.xfs_path, fs, c)) as f:
            cfgl = [x.strip() for x in f.readlines()]
        elif os.path.isfile('%s/fs/%s/cfg/%s' % (self.xfs_path, fs, c)):
          cfgl = [c]
        else:
          return
    # fs will be well defined as a single fs.
    # cfgl will be a list of configs assigned to that fs.
    if fs in self.fsconfigs:
      self.fsconfigs[fs].extend([c for c in cfgl
                                 if c not in self.fsconfigs[fs]])
    else:
      self.fsconfigs[fs] = cfgl
    return

  def expand_aliases(self):
    """Expands some explicit aliases of gce-xfstests test options.

    This function expands the short-hand forms of certain aliases
    into the explicit -c/-g options.

    The main reason for this is so that process_configs can find the '-c 4k'
    hidden inside of the 'smoke' option. Doing this to 'full' and 'quick'
    is unnecessary, as they only expand into '-g' options which can be left
    to the gce-xfstests command itself.
    """
    if 'smoke' in self.extra_cmds:
      self.expandedopts.append('smoke')
      self.extra_cmds[:] = [x for x in self.extra_cmds if x != 'smoke']
      self.extra_cmds[0:0] = ['-c', '4k', '-g', 'quick']

### end class LTMParser
