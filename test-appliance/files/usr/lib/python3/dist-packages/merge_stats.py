#!/usr/bin/python3

import argparse
import sys
import xml.etree.ElementTree as ET
import get_stats
import diff_stats
from gen_results_summary import TestStats
from junitparser import JUnitXml, Property, Properties, Failure, Error, Skipped


def merge_stats(stats1, stats2):
    """Merges stats2 into stats1"""
    for cfg in stats2:
        if cfg not in stats1:
            stats1[cfg] = {}

        for test_name in stats2[cfg]:
            if test_name not in stats1[cfg]:
                stats1[cfg][test_name] = TestStats()
            stats1[cfg][test_name].failed  += stats2[cfg][test_name].failed
            stats1[cfg][test_name].skipped += stats2[cfg][test_name].skipped
            stats1[cfg][test_name].error   += stats2[cfg][test_name].error
            stats1[cfg][test_name].total   += stats2[cfg][test_name].total

    return stats1

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('stats_file', help='First stats file', type=str)
    parser.add_argument('stats_files_merge', nargs='+', help='List of stats files to merge', type=str)
    parser.add_argument('--outfile', default='merged_stats.xml', help='Output xml file', type=str)
    args = parser.parse_args()

    stats = diff_stats.read_stats(args.stats_file)

    for file in args.stats_files_merge:
        stats_merge = diff_stats.read_stats(file)
        stats = merge_stats(stats, stats_merge)

    get_stats.write_stats(stats, args.outfile)


if __name__ == "__main__":
    main()
