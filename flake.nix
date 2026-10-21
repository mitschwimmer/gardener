{
  description = "Garden Go development environment";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = { nixpkgs, ... }: {
    devShells = nixpkgs.lib.genAttrs [ "x86_64-linux" "aarch64-linux" ] (system:
      let
        pkgs = nixpkgs.legacyPackages.${system};
      in {
        default = pkgs.mkShell {
          packages = with pkgs; [
            go_1_27
            gopls
            gnumake
            gcc
            git
            fish
            sops
            age
          ];

          GOTOOLCHAIN = "local";
          CGO_ENABLED = "1";
        };
      });
  };
}
