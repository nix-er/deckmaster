# Options and systemd user unit shared by the NixOS and home-manager modules.
{
  lib,
  pkgs,
  package,
}:
let
  inherit (lib) mkOption types;
in
{
  options = {
    package = mkOption {
      type = types.package;
      default = package;
      defaultText = lib.literalExpression "deckmaster.packages.\${system}.default";
      description = "The deckmaster package to use.";
    };

    deck = mkOption {
      type = types.nullOr types.str;
      default = null;
      example = "~/.config/deckmaster/main.deck";
      description = ''
        Deck file to load. When set, deckmaster runs as a systemd user service
        in the graphical session. A leading `~` is expanded by deckmaster.
      '';
    };

    brightness = mkOption {
      type = types.ints.between 0 100;
      default = 80;
      description = "Brightness in percent.";
    };

    keyboard = mkOption {
      type = types.enum [
        "auto"
        "wayland"
        "x11"
        "uinput"
        "none"
      ];
      default = "auto";
      description = "Keyboard emulation backend.";
    };

    device = mkOption {
      type = types.nullOr types.str;
      default = null;
      description = "Serial number of the device to use, if there is more than one.";
    };

    sleep = mkOption {
      type = types.nullOr types.str;
      default = null;
      example = "10m";
      description = "Turn the display off after this long without input.";
    };

    extraArgs = mkOption {
      type = types.listOf types.str;
      default = [ ];
      example = [ "-verbose" ];
      description = "Extra command line arguments.";
    };
  };

  # The [Unit]/[Service]/[Install] sections, in home-manager's attribute form.
  unit = cfg: {
    Unit = {
      Description = "Deckmaster, a Stream Deck controller";
      PartOf = [ "graphical-session.target" ];
      After = [ "graphical-session.target" ];
    };
    Service = {
      ExecStart = lib.escapeShellArgs (
        [
          (lib.getExe cfg.package)
          "-deck"
          cfg.deck
          "-brightness"
          (toString cfg.brightness)
          "-keyboard"
          cfg.keyboard
        ]
        ++ lib.optionals (cfg.device != null) [
          "-device"
          cfg.device
        ]
        ++ lib.optionals (cfg.sleep != null) [
          "-sleep"
          cfg.sleep
        ]
        ++ cfg.extraArgs
      );
      ExecReload = "${pkgs.coreutils}/bin/kill -HUP $MAINPID";
      Restart = "on-failure";
      RestartSec = 3;
    };
    Install.WantedBy = [ "graphical-session.target" ];
  };
}
