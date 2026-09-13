#include "GeoCoveragePinSubsystem.h"

bool UGeoCoveragePinSubsystem::IsPinned(const FString& Key) const
{
    if (Key.IsEmpty())
    {
        return false;
    }
    return Pins.ContainsByPredicate([&Key](const FGeoCoveragePin& Pin) { return Pin.Key == Key; });
}

bool UGeoCoveragePinSubsystem::TogglePin(const FString& Key, const FString& Label)
{
    if (Key.IsEmpty())
    {
        return false;
    }
    const int32 Existing = Pins.IndexOfByPredicate([&Key](const FGeoCoveragePin& Pin) { return Pin.Key == Key; });
    if (Existing != INDEX_NONE)
    {
        Pins.RemoveAt(Existing);
        OnPinsChanged.Broadcast();
        return false;
    }
    if (Pins.Num() >= MaxPins)
    {
        Pins.RemoveAt(0);
    }
    FGeoCoveragePin Pin;
    Pin.Key = Key;
    Pin.Label = Label.IsEmpty() ? Key : Label;
    Pins.Add(MoveTemp(Pin));
    OnPinsChanged.Broadcast();
    return true;
}

void UGeoCoveragePinSubsystem::Unpin(const FString& Key)
{
    const int32 Existing = Pins.IndexOfByPredicate([&Key](const FGeoCoveragePin& Pin) { return Pin.Key == Key; });
    if (Existing == INDEX_NONE)
    {
        return;
    }
    Pins.RemoveAt(Existing);
    OnPinsChanged.Broadcast();
}
