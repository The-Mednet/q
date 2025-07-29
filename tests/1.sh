#!/bin/sh

python tests/test_smtp.py \
    --from "Dr. Brian Adamski Jr. <brian@joinmednet.org>" \
    --to "Dr. Bobonski Smith Sr. <b@smada.org>" \
    --campaign 123 \
    --user 488708 \
    --subject "Get Expert Answers to Complex Clinical Questions" \
    --file tests/1.txt

