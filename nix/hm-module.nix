# home-manager module: runs deckmaster as a systemd user service. The udev
# rules that give the user access to the device need system configuration,
# e.g. programs.deckmaster.enable in the NixOS module.
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
    deck = lib.mkOption {
      type = lib.types.str;
      example = "~/.config/deckmaster/main.deck";
      description = "Deck file to load. A leading `~` is expanded by deckmaster.";
    };
  };

  config = lib.mkIf cfg.enable {
    home.packages = [ cfg.package ];
    systemd.user.services.deckmaster = service.unit cfg;
  };
}
