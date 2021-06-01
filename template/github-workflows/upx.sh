#!/usr/bin/env bash

for FILE in dist/Teemo*/*; do
    du -sh ${FILE}
    upx ${FILE}
    du -sh ${FILE}
done