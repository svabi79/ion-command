#pragma once

#include "CoreMinimal.h"
#include "Subsystems/GameInstanceSubsystem.h"
#include "IonCockpitPanelSubsystem.generated.h"

// One colored text cell of a cockpit panel row.
struct FIonCockpitPanelCell
{
    FString Text;
    FLinearColor Color = FLinearColor::White;
};

struct FIonCockpitPanelRow
{
    FString Label;
    TArray<FIonCockpitPanelCell> Cells;
};

struct FIonCockpitPanelModel
{
    FString Title;
    TArray<FIonCockpitPanelRow> Rows;
};

DECLARE_DELEGATE_RetVal(FIonCockpitPanelModel, FIonCockpitPanelProvider);

// Registration point for domain modules to contribute read-only instrument
// panels to the cockpit HUD without the UI module knowing their vocabulary.
UCLASS()
class IONCOMMANDUI_API UIonCockpitPanelSubsystem final : public UGameInstanceSubsystem
{
    GENERATED_BODY()

public:
    int32 RegisterProvider(FIonCockpitPanelProvider Provider)
    {
        const int32 Handle = NextHandle++;
        Providers.Add(Handle, MoveTemp(Provider));
        return Handle;
    }

    void UnregisterProvider(int32 Handle) { Providers.Remove(Handle); }

    TArray<FIonCockpitPanelModel> CollectPanels() const
    {
        TArray<FIonCockpitPanelModel> Panels;
        for (const TPair<int32, FIonCockpitPanelProvider>& Pair : Providers)
        {
            if (Pair.Value.IsBound()) Panels.Add(Pair.Value.Execute());
        }
        return Panels;
    }

private:
    TMap<int32, FIonCockpitPanelProvider> Providers;
    int32 NextHandle = 1;
};
