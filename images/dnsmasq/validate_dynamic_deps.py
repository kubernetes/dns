#/usr/bin/env python3

# Copyright 2022 The Kubernetes Authors.
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
# http://www.apache.org/licenses/LICENSE-2.0
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# This script requires python 3.8+
import os
import shutil
import subprocess
import pathlib
import sys
from typing import List

IMAGE_DIR = '.test-images'


def replace_img_special_chars(image_ref: str) -> str:
    """Replace container image name characters that cannot be a part of container names or file names."""
    return image_ref.lower().replace('/', '_').replace(':', '_')


def relative_to_root(path: pathlib.Path) -> pathlib.Path:
    path = pathlib.Path(path)
    return path.relative_to(path.anchor)


def detect_architecture(cwd: pathlib.Path, target_binary: pathlib.Path) -> str:
    """Read the architecture of the ELF file target_binary using the file command"""
    result = subprocess.run(['file', target_binary],
                            capture_output=True,
                            text=True,
                            cwd=cwd)
    if len(result.stdout.split(",")) == 0:
        # File not found
        return ""
    return result.stdout.split(",")[1].strip()


def untar_container_image(image_ref: str, output_dir: pathlib.Path):
    """Create a docker container using image_ref use tar to extract the filesystem to output_dir."""
    container_name = replace_img_special_chars(image_ref)
    subprocess.run(['docker', 'create', '--name', container_name, image_ref],
                   capture_output=True)
    subprocess.run(f'docker export {container_name} | tar x',
                   shell=True,
                   cwd=output_dir,
                   capture_output=True)
    subprocess.run(['docker', 'rm', container_name], capture_output=True)


def detect_dependencies(cwd: pathlib.Path,
                        target_binary: pathlib.Path) -> [str]:
    """Use objdump to read the ELF headers for target_binary and return a
    list of .so files.

    objdump is used instead of ldd because it reads the binary's contents
    instead of trying to run the binary. This means that it can be used on
    binaries with different architectures than the host machine.

    Using ldd on an x86 machine against x86 binary:
    ❯ ldd arm-container-test/k8s-dns-dnsmasq-amd64-1.22.3/usr/sbin/dnsmasq
        linux-vdso.so.1 (0x00007ffc3b9f5000)
        libc.so.6 => /lib/x86_64-linux-gnu/libc.so.6 (0x00007fb2f44f8000)
        /lib64/ld-linux-x86-64.so.2 (0x00007fb2f4748000)

    Using ldd x86 machine against an ARM binary:
    ❯ ldd arm-container-test/k8s-dns-dnsmasq-arm64-1.22.3/usr/sbin/dnsmasq
        not a dynamic executable
    """
    result = subprocess.run(['objdump', '--private-headers', target_binary],
                            capture_output=True,
                            text=True,
                            cwd=cwd)
    deps = []
    for line in result.stdout.splitlines():
        if "NEEDED" in line:
            deps.append(line.split()[1])
    return deps


def find_dependencies_with_name(root_dir: pathlib.Path, dependency_name: str) -> \
    List[pathlib.Path]:
    """Search for the first file called dependency_name within root_dir. """
    return list(root_dir.glob(f'**/{dependency_name}'))


def resolve_container_link(root_dir: pathlib.Path,
                           link: pathlib.Path) -> pathlib.Path:
    """Resolve the provided link, make absolute paths relative to root_dir."""
    resolved = os.readlink(link)

    if str(resolved).startswith('/'):
        return root_dir.joinpath(relative_to_root(resolved))
    return link.resolve().relative_to(root_dir.resolve())


def main(image_ref: str, target_binary: str, clean=True):
    print(f"Analyzing the binary {target_binary} from {image_ref}")
    container_name = replace_img_special_chars(image_ref)
    output_dir = pathlib.Path(IMAGE_DIR, container_name)
    target_binary = relative_to_root(pathlib.Path(target_binary))

    output_dir.mkdir(parents=True, exist_ok=True)
    untar_container_image(image_ref, output_dir)
    target_binary_arch = detect_architecture(output_dir, target_binary)
    deps = detect_dependencies(output_dir, target_binary)

    seen = set()
    missing = set()
    wrong_arch = {}
    while deps:
        current = deps.pop()

        if current in seen:
            continue
        seen.add(current)

        dep_paths = find_dependencies_with_name(output_dir, current)

        if not dep_paths:
            missing.add(current)
            continue

        for dep_path in dep_paths:

            if dep_path.is_symlink():
                dep_path = resolve_container_link(output_dir, dep_path)

            dep_arch = detect_architecture(output_dir, dep_path)

            if dep_arch != target_binary_arch:
                wrong_arch[dep_path] = dep_arch
                continue

            current_dependencies = detect_dependencies(output_dir, dep_path)
            deps.extend(current_dependencies)

    print("Dependencies:")
    for dep in seen:
        print(dep)
    print()

    exit_code = 0
    if wrong_arch:
        exit_code = 1
        print("Wrong Architecture:")
        for dep, arch in wrong_arch.items():
            print(f"{dep} - {arch}")
        print()

    if missing:
        exit_code = 1
        print("Missing Dependencies:")
        for miss in missing:
            print(miss)
        print()

    if exit_code == 0:
        print(f"All dependencies for {target_binary} are present")

    if clean:
        shutil.rmtree(output_dir)

    sys.exit(exit_code)


if __name__ == '__main__':
    import argparse

    parser = argparse.ArgumentParser(
        description=
        "A script to validate check if all of a binary's dynamically linked dependencies are present in the container"
    )
    parser.add_argument("--image",
                        help="A docker image to download and analyze",
                        required=True)
    parser.add_argument("--target-bin",
                        help="The binary to analyze within the IMAGE",
                        required=True)
    parser.add_argument("--skip-clean",
                        action="store_false",
                        help="Do not clean up the intermediate artifacts")

    args = parser.parse_args()
    main(args.image, args.target_bin, clean=args.skip_clean)
