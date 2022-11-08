#!/usr/bin/python3

import argparse
import sys
from gen_results_summary import get_property, get_testsuite_stats, get_results
from junitparser import JUnitXml, Property, Properties, Failure, Error, Skipped

try:
    from lxml import etree
except ImportError:
    from xml.etree import ElementTree as etree


# reports is list of results from each xml file
# stats[cfg] = cfg_stats
# cfg_stats[test] = TestStats()
def get_stats_from_dir(results_dir):
    """From a results dir, return a list of reports and test statistics"""
    reports = []
    stats = {}
    for filename in get_results(results_dir):
        reports.append(JUnitXml.fromfile(filename))

    if len(reports) == 0:
        sys.stderr.write(f'Error: could not find any reports in {results_dir}')
        return None

    for testsuite in reports:
        cfg = get_property(testsuite.properties(), 'TESTCFG') or get_property(testsuite.properties(), 'FSTESTCFG')
        if cfg in stats:
            sys.stderr.write(f'Found duplicate config {cfg}')
            return None
        stats[cfg] = get_testsuite_stats(testsuite)

    return stats

# writes all configs into single output file
# condensing into entries of test->(failed, skipped, error, total)
# this will let us store stats and easily merge from other runs
# without having to reprocess everything
def write_stats(s, output_file):
    """Write the test statistics to a file"""
    root = etree.Element("configs")
    for cfg in s:
        cfg_element = etree.SubElement(root, "config", name=cfg)
        for test_name in s[cfg]:
            test = s[cfg][test_name]
            etree.SubElement(cfg_element, "test", name=test_name, failed=str(test.failed), skipped=str(test.skipped), error=str(test.error), total=str(test.total))

    tree = etree.ElementTree(root)
    etree.indent(tree, space="\t", level=0)
    tree.write(output_file, encoding='utf-8')

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('results_dir', help='Results directory to process', type=str)
    parser.add_argument('--outfile', help='Diff output file', default='./stats.xml', type=str)
    args = parser.parse_args()

    stats = get_stats_from_dir(args.results_dir)

    if stats == None:
        return -1

    write_stats(stats, args.outfile)

if __name__ == "__main__":
    main()
