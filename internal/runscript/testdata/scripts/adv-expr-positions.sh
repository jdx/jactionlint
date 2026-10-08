echo ${{ matrix.a }}
echo "${{ matrix.b }}-${{ matrix.c }}" 'literal ${{ matrix.d }}'
${{ matrix.tool }} --flag
tool ${{ matrix.flag }} value
tool --opt=${{ matrix.v }}
echo ${{
  github.event.head_commit.message
}} done
echo ${{ format('{0}}}', github.ref) }} after
echo '${{ "unterminated' 
