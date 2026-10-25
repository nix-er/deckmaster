# NixOS module: installs deckmaster and its udev rules, and optionally runs it
# as a systemd user service for every user with a graphical session.
self:
{
  config,
  lib,
  pkgs,
  ...
}:
let
  cfg = config.programs.deckmaster;
  service = import ./service.nix {
    inherit lib pkgs;
    package = self.packages.${pkgs.stdenv.hostPlatform.system}.default;
  };
  unit = service.unit cfg;
in
{
  options.programs.deckmaster = service.options // {
    enable = lib.mkEnableOption "deckmaster, an application to control Elgato Stream Decks";
  };

  config = lib.mkIf cfg.enable {
    environment.systemPackages = [ cfg.package ];
    services.udev.packages = [ cfg.package ];

    systemd.user.services.deckmaster = lib.mkIf (cfg.deck != null) {
      description = unit.Unit.Description;
      partOf = unit.Unit.PartOf;
      after = unit.Unit.After;
      wantedBy = unit.Install.WantedBy;
      serviceConfig = unit.Service;
    };
  };
}
