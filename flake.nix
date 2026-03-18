{
  description = "kestrel (kest) CLI dev environment";
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    gomod2nix = {
      url = "github:nix-community/gomod2nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };
  outputs =
    {
      nixpkgs,
      gomod2nix,
      ...
    }:
    let
      supportedSystems = [
        "aarch64-darwin"
        "arm64-darwin"
        "x86_64-darwin"
        "x86_64-linux"
      ];
      eachSystem = f: nixpkgs.lib.genAttrs supportedSystems (system: f (pkgsFor system));
      pkgsFor =
        system:
        let
          pkgs = import nixpkgs {
            inherit system;
            config.allowUnfree = true;
          };
        in
        pkgs.extend (
          nixpkgs.lib.composeManyExtensions [
            gomod2nix.overlays.default
          ]
        );
    in
    {

      packages = eachSystem (pkgs: {
        default = pkgs.buildGoApplication {
          pname = "kest";
          version = "0.1.0";
          src = ./.;
          modules = ./gomod2nix.toml;
        };
      });

      devShells = eachSystem (pkgs: {
        default = pkgs.mkShell {
          buildInputs = with pkgs; [
            terraform
            terraform-ls
            helm-ls
            go
            gotools
            golangci-lint
            go-tools
            gopls
            gomod2nix.packages.${system}.default
          ];
          shellHook = ''
            echo "kestrel devshell"
          '';
        };
      });
    };
}
