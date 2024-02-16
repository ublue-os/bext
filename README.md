# ublue bext

Create and manage systemd-sysexts.

## Warning (for now)

Due to an [upstream bug](https://github.com/NixOS/nixpkgs/issues/252620), we cannot ship nix-based sysexts with SELinux support, and this is bound to break fedora systems, since they require SELinux labels on `/usr` and it's subdirectories.

If you want to try out this project now, please either try it out on a VM, or disable SELinux (permanently, because sysexts persist cross-reboot) on your host.
