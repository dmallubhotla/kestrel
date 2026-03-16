{
  description = "deepak file for example-service";
  inputs = {
    nixpkgs.url = "github:NixOS/nixpkgs/nixpkgs-unstable";
  };
  outputs =
    {
      nixpkgs,
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
        let pkgs = import nixpkgs {
          inherit system;
          config.allowUnfree = true;
        };
        in
        pkgs.extend (
          nixpkgs.lib.composeManyExtensions []
        );
    in
    {

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
          ];
          shellHook = ''
            echo "In example-service wrapper devshell"
            unset DEVELOPER_DIR
          '';
        };
      });
    };
}
