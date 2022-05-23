## GCEXfstests Dashboard
## Author: Harshad Shirwadkar (harshadshirwadkar@gmail.com)

import os
import csv
import subprocess

from flask import Flask
from datetime import datetime
from junitparser import JUnitXml, Property, Properties, Failure, Error, Skipped

app = Flask(__name__)

mirror_dir = os.environ.get("LOCAL_MIRROR_PATH", "/tmp/mirror")
extracted_dir = os.environ.get("LOCAL_EXTRACT_PATH", "/tmp/extracted/")
results_gs_path = os.environ.get("RESULTS_GS_PATH", "")
uncategorized_category = "Uncategorized"

def results_header():
    return """<title>GCEXfstests Dashboard</title>
<h1>GCE XFSTests Dashboard</h1>
<hr>
<table width=100px><th><a href="/">Home</a></th><th><a href=/files>Browse</a></th></table>
<hr>
</br>
"""

def result_summary(testsuite):
    if testsuite.errors == 0 and testsuite.failures == 0:
        return ("lightgreen", "Passed at %s" % testsuite.timestamp[11:])
    return ("yellow", "Failed at %s" % testsuite.timestamp[11:])

def get_results(dirroot):
    """Return a list of files named results.xml in a directory hierarchy"""
    for dirpath, dirs, filenames in os.walk(dirroot):
        if 'results.xml' in filenames:
            yield dirpath + '/results.xml'

def get_property(props, key):
    """Return the value of the first property with the given name"""
    if props is None:
        return None
    for prop in props:
        if prop.name == key:
            return prop.value
    return None

def run_shell_command(cmd):
    return subprocess.check_output(cmd.split(' '), stderr = subprocess.STDOUT)

def gs_rsync(gs_path, mirror_path):
    output = run_shell_command("gsutil -m rsync %s %s" % (gs_path, mirror_path))
    return len(output.splitlines()) > 2

def extract_tarballs(mirror_path, extract_path):
    for dirpath, dirs, filenames in os.walk(mirror_path):
        for tarball in filenames:
            extract_dir = extract_path + "/" + os.path.basename(tarball)
            if os.path.isdir(extract_path + "/" + extract_dir):
                    continue
            run_shell_command("mkdir -p " + extract_dir)
            run_shell_command("tar -xf %s/%s -C %s" % (dirpath, tarball, extract_dir))

    for dirent in os.listdir(extract_path):
        if not os.path.isfile(mirror_path + "/" + dirent):
            run_shell_command("rm -rf %s/%s" % (mirror_path, dirent))

def setup_dirs():
    run_shell_command("mkdir -p %s" % mirror_dir)
    run_shell_command("mkdir -p %s" % extracted_dir)

@app.route("/favicon.ico")
def favicon_ico_handler():
    return "null"

@app.route("/sync")
def sync_handler():
    ret = gs_rsync(results_gs_path, mirror_dir)
    if ret != 0:
        extract_tarballs(mirror_dir, extracted_dir)
        return "Sync performed with gs://" + results_gs_path
    return "Already upto date"

class testresult:
    def __init__(self, report, link, dirpath, category):
        self.report = report
        self.link = link
        self.dirpath = dirpath
        self.category = category
        self.cfg = get_property(self.report.properties(), 'TESTCFG')
        self.timestamp = self.report.timestamp

    def __repr__(self):
        return "(Link = %s, dirpath = %s, category = %s, cfg = %s)" % (self.link, self.dirpath, self.category, self.cfg)

@app.route("/")
def root_handler():
    total_categories = set()
    testresults = []

    if results_gs_path == "":
        return "Results bucket not set."

    setup_dirs()
    sync_handler()

    for dirpath, dirs, filenames in os.walk(extracted_dir):
        if 'results.xml' in filenames:
            report = JUnitXml.fromfile(dirpath + '/results.xml')
            link = dirpath.split('/')[extracted_dir.count('/')]
            category = uncategorized_category
            if os.path.isfile(dirpath + "/../../../ltm-info"):
                with open(dirpath + "/../../../report") as f:
                    for line in f.readlines():
                        if line.startswith("CMDLINE") and "--watch" in line:
                            parts = line.split(' ')
                            category = ""
                            parse_state = ""
                            repo = ""
                            branch = ""
                            for part in parts:
                                if parse_state == "repo":
                                    repo = part
                                elif parse_state == "branch":
                                    branch = part
                                if part == "--repo":
                                    parse_state = "repo"
                                elif part == "--watch":
                                    parse_state = "branch"
                                else:
                                    parse_state = ""
                            category = "repo: %s, branch: %s" % (repo, branch)
            total_categories.add(category)
            testresults.append(testresult(report, link, dirpath, category))

    out = results_header()

    for category in sorted(total_categories, reverse = True):
        table = {}
        configs = set()
        for result in testresults:
            if result.category != category:
                continue
            date = result.timestamp[0:10]
            if date not in table:
                table[date] = {}
            if result.cfg not in table[date]:
                table[date][result.cfg] = []
            configs.add(result.cfg)
            table[date][result.cfg].append(result)

        out += "<h3>%s</h3>" % category
        out += "<table><tr><th>Date</th>"
        for cfg in sorted(configs):
            out += "<th>%s</th>" % cfg
        out += "</tr>"
        last_date = ""
        print_timestamp = False

        for timestamp in sorted(table.keys(), reverse = True):
            max_items = 0
            print_timestamp = False
            if last_date != timestamp[0:10]:
                print_timestamp = True
                last_date = timestamp

            for cfg in table[timestamp].keys():
                max_items = max(max_items, len(table[timestamp][cfg]))

            for i in range(0, max_items):
                out += "<tr>"
                if print_timestamp:
                    out += "<td>%s</td>" % timestamp
                    print_timestamp = False
                else:
                    out += "<td></td>"
                for cfg in sorted(configs):
                    if cfg in table[timestamp] and i < len(table[timestamp][cfg]):
                        summary = result_summary(table[timestamp][cfg][i].report)
                        out += "<td bgcolor=%s><a href=/files/%s>%s</a></td>" % (summary[0], table[timestamp][cfg][i].link, summary[1])
                    else:
                        out += "<td bgcolor=lightgray></td>"
                out += "</tr>"
        out += "</table>"

    return out

@app.route("/files/<path:path>")
def file_browser_handler(path):
    out = results_header()
    out += "<pre>"
    if path != "/":
        parts = path.split('/')
        print(str(parts))
        out += "<a href=/files/%s>..</a>\n" % ('/'.join(parts[:(len(parts) - 1)]))
    if os.path.isfile(extracted_dir + path):
        out += "<hr>"
        with open(extracted_dir + path, "r") as f:
            out += f.read()
    else:
        for dirent in os.listdir(extracted_dir + path):
            if os.path.isdir(extracted_dir + path + "/" + dirent):
                out += "<a href=/files/%s/%s>%s/</a><br>" % (path, dirent, dirent)
        for dirent in os.listdir(extracted_dir + path):
            if os.path.isfile(extracted_dir + path + "/" + dirent):
                out += "<a href=/files/%s/%s>%s</a><br>" % (path, dirent, dirent)
    out += "</pre>"

    return out

@app.route("/files/")
def files_browser():
    return file_browser_handler("/")

if __name__ == "__main__":
    app.run(debug=True, host="0.0.0.0", port=int(os.environ.get("PORT", 8080)))
