#!/usr/bin/python3

import sys
import argparse
from junitparser import JUnitXml, TestSuite
from get_stats import get_stats_from_dir

parser = argparse.ArgumentParser()
parser.add_argument('results', help='Results directory')
parser.add_argument('config', help='Test config')
parser.add_argument('test', help='Test name you are intersted in')
parser.add_argument('--fail', help='Expected number of failures', type=int, required=False)
parser.add_argument('--skip', help='Expected number of skips', type=int, required=False)
parser.add_argument('--error', help='Expected number of errors', type=int, required=False)
parser.add_argument('--total', help='Expected number of total tests', type=int, required=False)
args = parser.parse_args()

results_stats = get_stats_from_dir(args.results)

ret=0
if args.fail is not None and args.fail != results_stats[args.config][args.test].failed:
    print(f"Error ({args.test}): expected {args.fail} failures but selftest had {results_stats[args.config][args.test].failed} failures.")
    ret=1

if args.skip is not None and args.skip != results_stats[args.config][args.test].skipped:
    print(f"Error ({args.test}): expected {args.skip} skips but selftest had {results_stats[args.config][args.test].skipped} skips.")
    ret=1

if args.error is not None and args.error != results_stats[args.config][args.test].error:
    print(f"Error ({args.test}): expected {args.error} errors but selftest had {results_stats[args.config][args.test].error} errors.")
    ret=1

if args.total is not None and args.total != results_stats[args.config][args.test].total:
    print(f"Error ({args.test}): expected {args.total} total tests but selftest had {results_stats[args.config][args.test].total} tests.")
    ret=1

sys.exit(ret)
