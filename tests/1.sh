#!/bin/sh

python tests/test_smtp.py \
    --from "Dr. Brian Adamski Jr. <brian@mail.joinmednet.org>" \
    --to "Dr. Brian Adams <b@smada.org>" \
    --campaign 123 \
    --user 60952 \
    --subject "$1" \
    --file tests/1.txt

