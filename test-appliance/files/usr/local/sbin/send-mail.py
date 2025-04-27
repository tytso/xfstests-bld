#!/usr/bin/python3
import os
import sys
import argparse
import sendgrid
from sendgrid.helpers.mail import *

def main(argv):
    if len(argv) < 2:
        print("Usage: send-mail.py infile")
        sys.exit(1)
    sendgrid_api_key = os.environ.get('SENDGRID_API_KEY')
    if sendgrid_api_key is None:
        print("missing Sendgrid API key")
        sys.exit(1)

    parser = argparse.ArgumentParser(description='Send mail using Sendgrid.')
    parser.add_argument('--sender', help='from address')
    parser.add_argument('-f', '--file', help='input file')
    parser.add_argument('-s', '--subject', help='subject line',
                        default='Report')
    parser.add_argument('dest', help='Destination address')
    args = parser.parse_args()

    sg = sendgrid.SendGridAPIClient(api_key=sendgrid_api_key)
    receivers = args.dest.split(',')

    if args.sender is None:
        from_email = Email(receivers[0])
    else:
        from_email = Email(args.sender)
    subject = args.subject
    if args.file is None:
        file = sys.stdin
    else:
        file = open(args.file, 'r')
    content = Content("text/plain", file.read())
    file.close()
    mail = Mail(from_email, receivers, subject, content)
    response = sg.client.mail.send.post(request_body=mail.get())
    status = response.status_code
    if status // 100 != 2:
        print(status)
        print(response.body)
        print(response.headers)
        sys.exit(1)

if __name__ == "__main__":
       main(sys.argv)
