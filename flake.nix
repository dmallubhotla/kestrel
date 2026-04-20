{
  description = "kestrel (kest) CLI dev environment";
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
    gomod2nix = {
      url = "github:nix-community/gomod2nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };
    treefmt-nix = {
      url = "github:numtide/treefmt-nix";
      inputs.nixpkgs.follows = "nixpkgs";
    };
  };
  outputs =
    {
      self,
      nixpkgs,
      gomod2nix,
      ...
    }@inputs:
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
            kestOverlay
          ]
        );
      treefmtEval = eachSystem (pkgs: inputs.treefmt-nix.lib.evalModule pkgs ./treefmt.nix);
      kestOverlay = final: _prev: {
        kest = final.buildGoApplication {
          pname = "kest";
          version = "0.1.0";
          src = ./.;
          modules = ./gomod2nix.toml;
          nativeBuildInputs = [ final.installShellFiles ];
          postInstall = ''
            mv $out/bin/kestrel $out/bin/kest
            installShellCompletion --cmd kest \
              --bash <($out/bin/kest completion bash) \
              --zsh <($out/bin/kest completion zsh) \
              --fish <($out/bin/kest completion fish)
          '';
        };
      };
    in
    {

      overlays.default = nixpkgs.lib.composeManyExtensions [
        gomod2nix.overlays.default
        kestOverlay
      ];

      packages = eachSystem (pkgs: {
        default = pkgs.kest;
      });

      formatter = eachSystem (pkgs: treefmtEval.${pkgs.stdenv.hostPlatform.system}.config.build.wrapper);
      checks = eachSystem (pkgs: {
        formatting = treefmtEval.${pkgs.stdenv.hostPlatform.system}.config.build.check self;
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
            gomod2nix.packages.${pkgs.stdenv.hostPlatform.system}.default
          ];
          shellHook = ''
            echo "kestrel devshell"
          '';
        };
      });
    };
}
