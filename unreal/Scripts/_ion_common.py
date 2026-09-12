import unreal


ROOT = "/Game/ION"


def ensure_directory(path: str) -> None:
    if not unreal.EditorAssetLibrary.does_directory_exist(path):
        unreal.EditorAssetLibrary.make_directory(path)


def load_class(module: str, class_name: str):
    result = unreal.load_class(None, f"/Script/{module}.{class_name}")
    if result is None:
        raise RuntimeError(f"C++ class is not available: {module}.{class_name}")
    return result


def save_asset(path: str) -> None:
    if not unreal.EditorAssetLibrary.save_asset(path, only_if_is_dirty=False):
        raise RuntimeError(f"Could not save asset: {path}")


def open_empty_rebuild_level(path: str) -> None:
    """Leave the command deck so deleting/rebuilding referenced assets cannot
    assert. UE 5.8 LevelEditorSubsystem.NewLevel refuses a destination that
    already exists ('Failed to validate the destination'); an interrupted
    prior run leaves these transient maps on disk, so reuse them.
    """
    if unreal.EditorAssetLibrary.does_asset_exist(path):
        if not unreal.EditorLevelLibrary.load_level(path):
            raise RuntimeError(f"Could not load leftover rebuild level {path}")
    else:
        created = unreal.EditorLevelLibrary.new_level(path)
        if not created:
            if not (
                unreal.EditorAssetLibrary.does_asset_exist(path)
                and unreal.EditorLevelLibrary.load_level(path)
            ):
                raise RuntimeError(f"Could not create transient rebuild level {path}")
    unreal.SystemLibrary.collect_garbage()


def discard_rebuild_level(path: str) -> None:
    if unreal.EditorAssetLibrary.does_asset_exist(path):
        unreal.EditorAssetLibrary.delete_asset(path)

