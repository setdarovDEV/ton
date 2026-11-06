package ton

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeRaw(t *testing.T) {
	raw := "0:6a29ea8f26e1cbef5b5448c759cb852f27cde7526f9cb211f1d472111c3e4f8f"
	friendly := "EQBqKeqPJuHL71tUSMdZy4UvJ83nUm-cshHx1HIRHD5Pj5YW" // mainnet bounceable
	got, err := NormalizeRaw(friendly)
	require.NoError(t, err)
	assert.Equal(t, raw, got)

	got, err = NormalizeRaw("0:6A29EA8F26E1CBEF5B5448C759CB852F27CDE7526F9CB211F1D472111C3E4F8F")
	require.NoError(t, err)
	assert.Equal(t, raw, got)

	_, err = NormalizeRaw("nonsense")
	assert.Error(t, err)
	_, err = NormalizeRaw("")
	assert.Error(t, err)
}

func TestToFriendly(t *testing.T) {
	raw := "0:6a29ea8f26e1cbef5b5448c759cb852f27cde7526f9cb211f1d472111c3e4f8f"
	mainnetBounce := ToFriendly(raw, false, true)
	assert.Equal(t, "EQBqKeqPJuHL71tUSMdZy4UvJ83nUm-cshHx1HIRHD5Pj5YW", mainnetBounce)
	testnetBounce := ToFriendly(raw, true, true)
	assert.NotEqual(t, mainnetBounce, testnetBounce)
	assert.Equal(t, "k", testnetBounce[:1])
	// round-trips
	back, err := NormalizeRaw(testnetBounce)
	require.NoError(t, err)
	assert.Equal(t, raw, back)
	assert.Equal(t, "junk", ToFriendly("junk", true, true))
}
