#!/bin/bash

set -e

. ./exports.sh

if [ $# -ne 1 ]; then
    echo "Usage: $0 <version>"
    exit 1
fi

admin_dir=/tmp/influx_admin_interface
influxdb_version=$1
rm -rf packages
mkdir packages

function package_admin_interface {
    [ -d $admin_dir ] || git clone https://github.com/influxdb/influxdb-js.git $admin_dir
    rvm rvmrc trust /tmp/influx_admin_interface/.rvmrc
    pushd $admin_dir
    git checkout .
    git pull --rebase

    bundle install
    bundle exec middleman build
    popd
}

function packae_source {
    rm -f influxd
    git ls-files --others  | egrep -v 'github|launchpad|code.google' > /tmp/influxdb.ignored
    echo "pkg/*" >> /tmp/influxdb.ignored
    echo "packages/*" >> /tmp/influxdb.ignored
    echo "build/*" >> /tmp/influxdb.ignored
    echo "out_rpm/*" >> /tmp/influxdb.ignored
    tar_file=influxdb-$influxdb_version.src.tar.gz
    tar -cvzf packages/$tar_file --exclude-vcs -X /tmp/influxdb.ignored *
    pushd packages
    # put all files in influxdb
    mkdir influxdb
    tar -xvzf $tar_file -C influxdb
    rm $tar_file
    tar -cvzf $tar_file influxdb
    popd
}

function package_files {
    if [ $# -ne 1 ]; then
        echo "Usage: $0 architecture"
        return 1
    fi

    rm -rf build
    mkdir build

    package_admin_interface

    mv influxd build/influxdb

    cp config.json.sample build/

    # cp -R src/admin/site/ build/admin/
    mkdir build/admin
    cp -R $admin_dir/build/* build/admin/

    cp -R scripts/ build/

    tar_file=influxdb-$influxdb_version.$1.tar.gz

    tar -czf $tar_file build/*

    mv $tar_file packages/

    # the tar file should use "./assets" but the deb and rpm packages should use "/opt/influxdb/current/admin"
    mv build/config.json.sample build/config.json
    sed -i.bak -e 's/"AdminAssetsDir.*/"AdminAssetsDir": "\/opt\/influxdb\/current\/admin\/",/' build/config.json
    rm build/config.json.bak
}

function build_packages {
    if [ $# -ne 1 ]; then
        echo "Usage: $0 architecture"
        return 1
    fi

    if [ $1 == "386" ]; then
        rpm_args="setarch i386"
        deb_args="-a i386"
    fi

    rm -rf out_rpm
    mkdir -p out_rpm/opt/influxdb/versions/$influxdb_version
    cp -r build/* out_rpm/opt/influxdb/versions/$influxdb_version
    pushd out_rpm
    $rpm_args fpm  -s dir -t rpm --after-install ../scripts/post_install.sh -n influxdb -v $influxdb_version . || exit $?
    mv *.rpm ../packages/
    fpm  -s dir -t deb $deb_args --after-install ../scripts/post_install.sh -n influxdb -v $influxdb_version . || exit $?
    mv *.deb ../packages/
    popd
}

function setup_version {
    echo "Changing version from dev to $influxdb_version"
    sha1=`git rev-list --max-count=1 HEAD`
    sed -i.bak -e "s/version = \"dev\"/version = \"$influxdb_version\"/" -e "s/gitSha = \"\"/gitSha = \"$sha1\"/" src/server/influxd.go
    sed -i.bak -e "s/REPLACE_VERSION/$influxdb_version/" scripts/post_install.sh
}

function revert_version {
    if [ -e src/server/influxd.go.bak ]; then
        rm src/server/influxd.go
        mv src/server/influxd.go.bak src/server/influxd.go
    fi

    if [ -e scripts/post_install.sh ]; then
        rm scripts/post_install.sh
        mv scripts/post_install.sh.bak scripts/post_install.sh
    fi

    echo "Changed version back to dev"
}

setup_version
packae_source
UPDATE=on ./build.sh && package_files amd64 && build_packages amd64
[ $on_linux == yes ] && CGO_ENABLED=1 GOARCH=386 UPDATE=on ./build.sh && package_files 386 && build_packages 386
revert_version
