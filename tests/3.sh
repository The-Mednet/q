#!/bin/sh

python tests/test_smtp.py \
    --from "Dr. Brian Adamski Jr. <brian@mail.joinmednet.org>" \
    --to "Dr. Brian Adams <b@smada.org>" \
    --invitation 123 \
    --email-type campaign\
    --dispatch 1 \
    --subject "$1" \
    --file tests/1.txt
