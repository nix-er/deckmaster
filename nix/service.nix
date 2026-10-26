# Options, systemd user units and assertions shared by the NixOS and
# home-manager modules. Each Stream Deck runs as its own deckmaster process:
# the top-level options describe the default instance (deckmaster.service),
# and every entry in `instances` adds a deckmaster-<name>.service.
{
  lib,
  pkgs,
  package,
}:
let
  inherit (lib) mkOption types;

  toml = pkgs.formats.toml { };

  # Options of one deckmaster process. `iconPacks` is added separately, since
  # named instances fall back to the top-level packs.
  instanceOptions = {
    deck = mkOption {
      type = types.nullOr types.str;
      default = null;
      example = "~/.config/deckmaster/main.deck";
      description = ''
        Deck file to load. When this or `decks` is set, deckmaster runs as a
        systemd user service in the graphical session. A leading `~` is
        expanded by deckmaster.
      '';
    };

    decks = mkOption {
      type = types.attrsOf toml.type;
      default = { };
      example = lib.literalExpression ''
        {
          main.keys = [
            { index = 0; widget.id = "clock"; }
            {
              index = 1;
              widget = { id = "button"; config.label = "Media"; };
              action.deck = "media.deck";
            }
          ];
          media.parent = "main.deck";
        }
      '';
      description = ''
        Deck files as Nix, keyed by name without `.deck`. They are written to
        one directory in the Nix store, so they can refer to each other by
        file name (`deck = "media.deck"`, `parent = "main.deck"`), and
        deckmaster starts with `main`. Relative icon paths resolve in that
        directory too, so use icon packs or absolute paths.
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
      description = ''
        Serial number of the device to use. Required when more than one
        instance runs; `deckmaster -device x` lists the connected devices.
      '';
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

  iconPacksExample = lib.literalExpression "[ deckmaster.packages.\${system}.simple-icons ]";

  # The default instance and the named ones as { <unit name> = instance; },
  # leaving out those without a deck.
  instancesOf =
    cfg:
    lib.filterAttrs (_: i: i.deck != null || i.decks != { }) (
      { deckmaster = cfg; } // lib.mapAttrs' (name: lib.nameValuePair "deckmaster-${name}") cfg.instances
    );

  deckDir =
    unitName: decks:
    pkgs.linkFarm "${unitName}-decks" (
      lib.mapAttrsToList (name: settings: {
        name = "${name}.deck";
        path = toml.generate "${name}.deck" settings;
      }) decks
    );

  # The [Unit]/[Service]/[Install] sections, in home-manager's attribute form.
  # The deck directory is part of ExecStart, so changing a deck restarts only
  # its instance.
  unit =
    cfg: unitName: i:
    let
      iconPacks = if i.iconPacks == null then cfg.iconPacks else i.iconPacks;
      deck = if i.decks != { } then "${deckDir unitName i.decks}/main.deck" else i.deck;
    in
    {
      Unit = {
        Description =
          if unitName == "deckmaster" then
            "Deckmaster, a Stream Deck controller"
          else
            "Deckmaster (${lib.removePrefix "deckmaster-" unitName})";
        PartOf = [ "graphical-session.target" ];
        After = [ "graphical-session.target" ];
      };
      Service = {
        ExecStart = lib.escapeShellArgs (
          [
            (lib.getExe cfg.package)
            "-deck"
            deck
            "-brightness"
            (toString i.brightness)
            "-keyboard"
            i.keyboard
          ]
          ++ lib.optionals (i.device != null) [
            "-device"
            i.device
          ]
          ++ lib.optionals (i.sleep != null) [
            "-sleep"
            i.sleep
          ]
          ++ i.extraArgs
        );
        Environment = lib.optionals (iconPacks != [ ]) [
          "DECKMASTER_ICON_PATH=${lib.concatMapStringsSep ":" (p: "${p}/share/deckmaster/icons") iconPacks}"
        ];
        ExecReload = "${pkgs.coreutils}/bin/kill -HUP $MAINPID";
        Restart = "on-failure";
        RestartSec = 3;
      };
      Install.WantedBy = [ "graphical-session.target" ];
    };
in
{
  options = instanceOptions // {
    package = mkOption {
      type = types.package;
      default = package;
      defaultText = lib.literalExpression "deckmaster.packages.\${system}.default";
      description = "The deckmaster package to use.";
    };

    iconPacks = mkOption {
      type = types.listOf types.package;
      default = [ ];
      example = iconPacksExample;
      description = ''
        Icon packs to look up button icons by name in, e.g. `icon = "firefox"`.
        Packs keep their icons in `share/deckmaster/icons/<pack>`. Instances
        use these unless they set their own.
      '';
    };

    instances = mkOption {
      type = types.attrsOf (
        types.submodule {
          options = instanceOptions // {
            iconPacks = mkOption {
              type = types.nullOr (types.listOf types.package);
              default = null;
              example = iconPacksExample;
              description = "Icon packs for this instance, instead of the top-level ones.";
            };
          };
        }
      );
      default = { };
      example = lib.literalExpression ''
        {
          plus = {
            device = "A00WA0000000";
            decks.main = { ... };
          };
          xl = {
            device = "A00NA0000000";
            deck = "~/.config/deckmaster/xl.deck";
          };
        }
      '';
      description = ''
        More deckmaster processes, one per Stream Deck, each running as
        `deckmaster-<name>.service`. They take the same options as the top
        level, which configures `deckmaster.service`.
      '';
    };
  };

  units = cfg: lib.mapAttrs (unit cfg) (instancesOf cfg);

  assertions =
    cfg:
    let
      instances = instancesOf cfg;
      several = lib.length (lib.attrNames instances) > 1;
    in
    lib.concatLists (
      lib.mapAttrsToList (unitName: i: [
        {
          assertion = i.decks == { } || i.decks ? main;
          message = "deckmaster: ${unitName} sets decks but no decks.main to start with.";
        }
        {
          assertion = i.deck == null || i.decks == { };
          message = "deckmaster: ${unitName} sets both deck and decks.";
        }
        {
          assertion = !several || i.device != null;
          message = "deckmaster: ${unitName} needs a device serial, since more than one instance runs.";
        }
      ]) instances
    );
}
