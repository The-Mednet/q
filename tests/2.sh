#!/bin/sh

python tests/test_smtp.py \
    --from "Dr. B. Adams <brian@mednetmail.org>" \
    --to "Some guy <b@smada.org>" \
    --campaign 123 \
    --user 456 \
    --subject "Get Expert Answers to Complex Clinical Questions" \
    --file tests/1.txt

