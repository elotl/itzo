#!/bin/sh

# upload file with cli:
# aws s3 cp itzo s3://itzo-download/ --acl public-read

s3_path="http://itzo-download.s3.amazonaws.com/itzo"
itzo_dir=/usr/local/bin
itzo_path=${itzo_dir}/itzo
rm $itzo_path
wget $s3_path -P $itzo_dir
chmod 755 $itzo_path
$itzo_path
