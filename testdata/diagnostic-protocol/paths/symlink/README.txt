via_link.gon is a symlink to real/target.gon.
If the checkout does not preserve the symlink, the harness must
recreate it: ln -sfn real/target.gon via_link.gon
