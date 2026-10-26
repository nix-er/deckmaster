# home-manager module: runs deckmaster as systemd user services, one per
# Stream Deck. The udev rules that give the user access to the device need
# system configuration, e.g. programs.deckmaster.enable in the NixOS module.
self:
{
  config,
  lib,
  pkgs,
  ...
}:
let
  cfg = config.services.deckmaster;
  service = import ./service.nix {
    inherit lib pkgs;
    package = self.packages.${pkgs.stdenv.hostPlatform.system}.default;
  };
in
{
  options.services.deckmaster = service.options // {
    enable = lib.mkEnableOption "deckmaster, an application to control Elgato Stream Decks";
  };

  config = lib.mkIf cfg.enable {
    assertions = service.assertions cfg;

    home.packages = [ cfg.package ];
    systemd.user.services = service.units cfg;
  };
}
