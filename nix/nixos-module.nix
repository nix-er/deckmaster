# NixOS module: installs deckmaster and its udev rules, and optionally runs it
# as systemd user services, one per Stream Deck, for every user with a
# graphical session.
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
in
{
  options.programs.deckmaster = service.options // {
    enable = lib.mkEnableOption "deckmaster, an application to control Elgato Stream Decks";
  };

  config = lib.mkIf cfg.enable {
    assertions = service.assertions cfg;

    environment.systemPackages = [ cfg.package ];
    services.udev.packages = [ cfg.package ];

    systemd.user.services = lib.mapAttrs (_: unit: {
      description = unit.Unit.Description;
      partOf = unit.Unit.PartOf;
      after = unit.Unit.After;
      wantedBy = unit.Install.WantedBy;
      serviceConfig = unit.Service;
      # Keep the session's PATH for exec actions; NixOS would replace it with
      # a minimal one.
      enableDefaultPath = false;
    }) (service.units cfg);
  };
}
