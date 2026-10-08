pip install requests
pip install requests==2.31.0 "urllib3>=2" 'idna~=3.0' six==1.*
pip3 install --upgrade pip setuptools wheel
python -m pip install -U pip
python3.11 -m pip install --user "pkg[extra]==1.0" -r requirements.txt -c constraints.txt
pip install --index-url https://example.com/simple --extra-index-url https://pypi.org/simple pkg
pip install -e . ./dist/*.whl ../x.tgz /abs/path.whl
pip install git+https://github.com/o/r.git@v1.2.3 git+https://github.com/o/r.git@main git+https://github.com/o/r.git@0123456789abcdef0123456789abcdef01234567
pip install pkg@https://example.com/pkg.whl https://example.com/other.whl#sha256=abc
pip install --require-hashes -r req.txt
pip install "pkg==${{ inputs.version }}" ${{ inputs.extra }} $PKGS "$(cat pkgs.txt)"
pip install requests; pip uninstall -y requests; pip --version; pip list
sudo pip install pkg
pip install -r requirements.txt --no-deps && pip check
