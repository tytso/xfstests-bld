#!/usr/bin/python3

import argparse
import sys
from gen_results_summary import TestStats
import xml.etree.ElementTree as ET
from junitparser import JUnitXml, Property, Properties, Failure, Error, Skipped


# s[cfg] = cfg_stats
# cfg_stats[test] = TestStats()
# consider s1 the baseline
def diff_stats(s1, s2, threshold, output_file, input_file1, input_file2):
    """Compare the statistics between two Stats, report regressions and unexpected results"""
    print(f"Writing results to {output_file}")

    skip_str=""
    error_str=""
    file = open(output_file, 'w')
    file.write(f'Regression check {input_file1} -> {input_file2}:\n\n')
    for cfg in s1.keys():
        if cfg not in s2.keys():
            file.write(f'***Warning: missing config {cfg} in {input_file2}***\n')

    for cfg in s2.keys():
        file.write(f'{cfg:-^45}\n')
        if cfg not in s1.keys():
            file.write(f'***Warning: missing config {cfg} in {input_file1}***\n')
            continue
        for test_name in s2[cfg]:
            test = s2[cfg][test_name]
            if test_name not in s1[cfg]:
                file.write(f'***Warning: {cfg}:{test_name} run on {input_file2} but not on {input_file1}***\n')
                continue
            if test.failed > 0:
                test_1 = s1[cfg][test_name]
                fail_rate_1 = 100.0 * test_1.failed / test_1.total
                fail_rate_2 = 100.0 * test.failed / test.total
                if fail_rate_2 >= fail_rate_1 + threshold:
                    file.write(f'{test_name}: {test_1.failed}/{test_1.total} ({fail_rate_1:.2f}%) -> {test.failed}/{test.total} ({fail_rate_2:.2f}%)\n')

            test_1 = s1[cfg][test_name]
            skip_rate_1 = 100.0 * test_1.skipped / test_1.total
            skip_rate_2 = 100.0 * test.skipped / test.total
            if skip_rate_1 != skip_rate_2:
                skip_str+=f'{cfg}:{test_name} skip rate changed {test_1.skipped}/{test_1.total} ({skip_rate_1:.2f}%) -> {test.skipped}/{test.total} ({skip_rate_2:.2f}%)\n'

            if test.error > 0:
                test_1 = s1[cfg][test_name]
                error_rate_1 = 100.0 * test_1.error / test_1.total
                error_rate_2 = 100.0 * test.error / test.total
                # always print error stats
                error_str+=f'{cfg}:{test_name} ERROR {test_1.error}/{test_1.total} ({error_rate_1:.2f})% -> {test.error}/{test.total} ({error_rate_2:.2f}%)\n'
        file.write('\n')

    if len(error_str) > 0:
        file.write('\n*** ERROR(S) occurred in new test set: ***\n')
        file.write(error_str)

    if len(skip_str) > 0:
        file.write('\n*** WARNING: skip rate changed between test sets: ***\n')
        file.write(skip_str)
    file.close()


def read_stats(input_file):
    """Read test statistics from file"""
    stats = {}
    tree = ET.parse(input_file)
    root = tree.getroot()

    for cfg_element in root.findall('config'):
        cfg = cfg_element.get('name')
        if cfg not in stats:
            stats[cfg] = {}
        for test_element in cfg_element.findall('test'):
            test = TestStats()

            name         = test_element.get('name')
            test.failed  = int(test_element.get('failed'))
            test.skipped = int(test_element.get('skipped'))
            test.error   = int(test_element.get('error'))
            test.total   = int(test_element.get('total'))

            stats[cfg][name] = test

    return stats


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('stats_file1', help='First stats file (baseline)', type=str)
    parser.add_argument('stats_file2', help='Second stats file (file to compare to baseline)', type=str)
    parser.add_argument('--outfile', help='Diff output file', default="stats.diff", type=str)
    parser.add_argument('--regression_threshold', help='Percent (int) increase needed in fail rate to determine regression', type=int, default=5)
    args = parser.parse_args()

    stats1 = read_stats(args.stats_file1)
    stats2 = read_stats(args.stats_file2)

    diff_stats(stats1, stats2, args.regression_threshold, args.outfile, args.stats_file1, args.stats_file2)


if __name__ == "__main__":
    main()
