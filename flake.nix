{
  description = "An application to control your Elgato Stream Deck on Linux";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs =
    { self, nixpkgs }:
    let
      systems = [
        "x86_64-linux"
        "aarch64-linux"
      ];
      forAllSystems = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
    in
    {
      packages = forAllSystems (pkgs: {
        deckmaster = pkgs.buildGo127Module {
          pname = "deckmaster";
          version = self.shortRev or self.dirtyShortRev or "dev";
          src = nixpkgs.lib.cleanSource ./.;

          # Update when go.mod/go.sum change: set to nixpkgs.lib.fakeHash, build, copy the hash.
          vendorHash = "sha256-kngnBr5zWe6m2WM6X42mFL/uKY9O1xZstkGWnBQwGFs=";

          ldflags = [
            "-s"
            "-w"
            "-X main.CommitSHA=${self.rev or self.dirtyRev or ""}"
          ];

          nativeBuildInputs = [ pkgs.makeWrapper ];

          postInstall = ''
            install -Dm644 udev/70-streamdeck.rules $out/lib/udev/rules.d/70-streamdeck.rules
            # deckmaster looks up its default fonts (Roboto) via XDG_DATA_DIRS,
            # and pastes with wl-copy on Wayland or xclip on X11
            wrapProgram $out/bin/deckmaster \
              --suffix XDG_DATA_DIRS : ${pkgs.roboto}/share \
              --suffix PATH : ${
                pkgs.lib.makeBinPath [
                  pkgs.wl-clipboard
                  pkgs.xclip
                ]
              }
          '';

          meta = {
            description = "An application to control your Elgato Stream Deck on Linux";
            homepage = "https://github.com/nix-er/deckmaster";
            license = nixpkgs.lib.licenses.mit;
            mainProgram = "deckmaster";
            platforms = nixpkgs.lib.platforms.linux;
          };
        };
        default = self.packages.${pkgs.stdenv.hostPlatform.system}.deckmaster;

        # Brand logos from Simple Icons, in their brand colors, packaged as a
        # Stream Deck icon pack. Use with the iconPacks module option, e.g.
        # icon = "firefox" in a deck.
        simple-icons = pkgs.stdenvNoCC.mkDerivation rec {
          pname = "deckmaster-simple-icons";
          version = "16.35.0";

          src = pkgs.fetchurl {
            url = "https://github.com/mackenly/simple-icons-stream-deck/releases/download/v${version}/com.mackenly.simpleiconsstreamdeck.streamDeckIconPack";
            hash = "sha256-dL6sYxxHmlpoUBlNCNQJDr/FI/tk5A+LS3FazXvjWKY=";
          };

          nativeBuildInputs = [ pkgs.unzip ];

          unpackPhase = ''
            unzip -q $src
          '';

          installPhase = ''
            dir=$out/share/deckmaster/icons/simple-icons
            mkdir -p $dir
            cp -r com.mackenly.simpleiconsstreamdeck.sdIconPack/icons com.mackenly.simpleiconsstreamdeck.sdIconPack/icons.json $dir
          '';

          meta = {
            description = "Simple Icons brand logos for deckmaster";
            homepage = "https://github.com/mackenly/simple-icons-stream-deck";
            # The logos are trademarks of their owners, see
            # https://github.com/simple-icons/simple-icons/blob/develop/DISCLAIMER.md
            license = nixpkgs.lib.licenses.cc0;
            platforms = nixpkgs.lib.platforms.all;
          };
        };
      });

      overlays.default = final: _prev: {
        deckmaster = self.packages.${final.stdenv.hostPlatform.system}.deckmaster;
      };

      nixosModules.default = import ./nix/nixos-module.nix self;
      homeManagerModules.default = import ./nix/hm-module.nix self;

      checks = forAllSystems (pkgs: {
        deckmaster = self.packages.${pkgs.stdenv.hostPlatform.system}.deckmaster;

        # Evaluates the NixOS module and checks it produces the user service.
        nixos-module =
          let
            system = nixpkgs.lib.nixosSystem {
              modules = [
                self.nixosModules.default
                {
                  nixpkgs.hostPlatform = pkgs.stdenv.hostPlatform.system;
                  boot.loader.grub.enable = false;
                  fileSystems."/".device = "nodev";
                  system.stateVersion = "25.11";
                  programs.deckmaster = {
                    enable = true;
                    deck = "~/main.deck";
                    sleep = "10m";
                    iconPacks = [ self.packages.${pkgs.stdenv.hostPlatform.system}.simple-icons ];
                  };
                }
              ];
            };
            unit = system.config.systemd.user.units."deckmaster.service".text;
          in
          assert nixpkgs.lib.assertMsg (nixpkgs.lib.hasInfix "DECKMASTER_ICON_PATH=" unit)
            "deckmaster.service doesn't set DECKMASTER_ICON_PATH";
          pkgs.writeText "deckmaster.service" unit;
      });

      devShells = forAllSystems (pkgs: {
        default = pkgs.mkShell {
          packages = with pkgs; [
            go_1_27
            gcc
            pkg-config
            golangci-lint
            goreleaser
            govulncheck
          ];
        };
      });

      formatter = forAllSystems (pkgs: pkgs.nixfmt);
    };
}
